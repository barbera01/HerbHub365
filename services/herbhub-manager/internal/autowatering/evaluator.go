package autowatering

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"HerbHub365/services/herbhub-manager/internal/messaging"
	"HerbHub365/services/herbhub-manager/internal/prometheus"
)

var (
	errFenceRevisionChanged = errors.New("publication fence: config revision changed")
	errFenceDisabled        = errors.New("publication fence: automatic watering disabled")
	errFencePlantDisabled   = errors.New("publication fence: plant disabled")
	errFencePolicyChanged   = errors.New("publication fence: policy changed")
	errMultipleSeries       = errors.New("multiple series")
	errKindPairing          = errors.New("invalid value/timestamp pairing")
)

const reservedKindLabelName = "_hh_kind"

type PromClient interface {
	QueryVectorAt(ctx context.Context, expr string, evaluationTime time.Time) ([]prometheus.Sample, error)
}

type WaterPublisher interface {
	PublishAutomaticWater(ctx context.Context, plant string, moisture float64, expiry time.Duration) (messaging.AutomaticWaterResult, error)
}

type Evaluator struct {
	store     *Store
	prom      PromClient
	publisher WaterPublisher

	mu     sync.Mutex
	run    bool
	fault  bool
	faultS string
	next   *time.Time

	done chan struct{}
}

func NewEvaluator(store *Store, prom PromClient, publisher WaterPublisher) *Evaluator {
	return &Evaluator{store: store, prom: prom, publisher: publisher, done: make(chan struct{})}
}

func (e *Evaluator) Start(ctx context.Context) {
	e.mu.Lock()
	if e.run {
		e.mu.Unlock()
		return
	}
	e.run = true
	e.mu.Unlock()

	go e.runLoop(ctx)
}

func (e *Evaluator) Wait() {
	<-e.done
}

func (e *Evaluator) Status() RuntimeStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	var next *time.Time
	if e.next != nil {
		n := e.next.UTC()
		next = &n
	}
	return RuntimeStatus{Running: e.run, Faulted: e.fault, Fault: e.faultS, NextEvaluationAt: next}
}

func (e *Evaluator) setNext(t *time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t == nil {
		e.next = nil
		return
	}
	v := t.UTC()
	e.next = &v
}

func (e *Evaluator) setFault(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fault = true
	e.faultS = strings.TrimSpace(err.Error())
	e.next = nil
}

func (e *Evaluator) clearRun() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.run = false
	e.next = nil
}

func (e *Evaluator) runLoop(ctx context.Context) {
	defer close(e.done)
	defer e.clearRun()

	if e.store == nil {
		e.setFault(errors.New("automatic watering store unavailable"))
		return
	}
	changes := e.store.Changes()
	doc := e.store.Snapshot()
	interval := time.Duration(doc.Config.EvaluationIntervalSeconds) * time.Second
	timer := time.NewTimer(interval)
	defer timer.Stop()
	next := time.Now().UTC().Add(interval)
	e.setNext(&next)

	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			doc = e.store.Snapshot()
			interval = time.Duration(doc.Config.EvaluationIntervalSeconds) * time.Second
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(interval)
			next = time.Now().UTC().Add(interval)
			e.setNext(&next)
		case <-timer.C:
			if err := e.EvaluateOnce(ctx); err != nil {
				e.setFault(err)
				log.Printf("autowatering evaluator fault: %v", err)
				return
			}
			doc = e.store.Snapshot()
			interval = time.Duration(doc.Config.EvaluationIntervalSeconds) * time.Second
			timer.Reset(interval)
			next = time.Now().UTC().Add(interval)
			e.setNext(&next)
		}
	}
}

func (e *Evaluator) EvaluateOnce(ctx context.Context) error {
	if e.store == nil {
		return errors.New("store unavailable")
	}
	doc := e.store.Snapshot()
	cfg := doc.Config
	revision := doc.ConfigRevision
	now := time.Now().UTC()

	for _, plant := range fixedPlants {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !cfg.Enabled {
			if err := e.recordDecision(plant, DecisionDisabled, now, nil, nil, "", ""); err != nil {
				return err
			}
			continue
		}
		plantCfg := cfg.Plants[plant]
		if !plantCfg.Enabled {
			if err := e.recordDecision(plant, DecisionDisabled, now, nil, nil, "", ""); err != nil {
				return err
			}
			continue
		}
		if e.prom == nil {
			if err := e.recordDecision(plant, DecisionInvalidMetric, now, nil, nil, "prometheus client unavailable", ""); err != nil {
				return err
			}
			continue
		}

		sel := buildSelector(cfg.MoistureMetric, cfg.PlantLabel, plantCfg.MetricLabelValue)
		atomicExpr := buildAtomicValueTimestampExpr(sel)
		qctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.PrometheusTimeoutSeconds)*time.Second)
		samples, err := e.prom.QueryVectorAt(qctx, atomicExpr, now)
		var sample prometheus.Sample
		var sourceTS time.Time
		if err == nil {
			sample, sourceTS, err = deriveSampleAndSourceTimestamp(samples, cfg.PlantLabel, plantCfg.MetricLabelValue)
		}
		cancel()
		if err != nil {
			if errors.Is(err, prometheus.ErrNoSeries) {
				if err := e.recordDecision(plant, DecisionNoMetric, now, nil, nil, "", ""); err != nil {
					return err
				}
				continue
			}
			if errors.Is(err, errMultipleSeries) {
				if err := e.recordDecision(plant, DecisionMultipleMetrics, now, nil, nil, "", ""); err != nil {
					return err
				}
				continue
			}
			if err := e.recordDecision(plant, DecisionInvalidMetric, now, nil, nil, sanitizeError(err), ""); err != nil {
				return err
			}
			continue
		}
		sample.Timestamp = sourceTS
		value := sample.Value
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			if err := e.recordDecision(plant, DecisionInvalidMetric, now, &sample.Timestamp, &value, "value outside [0,100]", ""); err != nil {
				return err
			}
			continue
		}
		if sample.Timestamp.IsZero() {
			if err := e.recordDecision(plant, DecisionInvalidMetric, now, nil, &value, "invalid source timestamp", ""); err != nil {
				return err
			}
			continue
		}
		if sample.Timestamp.After(now.Add(allowedFutureSampleSkew)) {
			if err := e.recordDecision(plant, DecisionFutureMetric, now, &sample.Timestamp, &value, "", ""); err != nil {
				return err
			}
			continue
		}
		if now.Sub(sample.Timestamp) > time.Duration(cfg.MaxMetricAgeSeconds)*time.Second {
			if err := e.recordDecision(plant, DecisionStaleMetric, now, &sample.Timestamp, &value, "", ""); err != nil {
				return err
			}
			continue
		}
		if value >= plantCfg.ThresholdPercent {
			if err := e.recordDecision(plant, DecisionHealthy, now, &sample.Timestamp, &value, "", ""); err != nil {
				return err
			}
			continue
		}

		state := e.store.Snapshot().Plants[plant]
		if state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
			if err := e.recordDecision(plant, DecisionCooldown, now, &sample.Timestamp, &value, "", ""); err != nil {
				return err
			}
			continue
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		var res messaging.AutomaticWaterResult
		pubErr := e.store.WithPublicationFence(func() error {
			latest := e.store.Snapshot()
			if latest.ConfigRevision != revision {
				return errFenceRevisionChanged
			}
			lcfg := latest.Config
			if !lcfg.Enabled {
				return errFenceDisabled
			}
			lplant := lcfg.Plants[plant]
			if !lplant.Enabled {
				return errFencePlantDisabled
			}
			if lcfg.CooldownSeconds != cfg.CooldownSeconds || lcfg.MessageExpirySeconds != cfg.MessageExpirySeconds || lcfg.MoistureMetric != cfg.MoistureMetric || lcfg.PlantLabel != cfg.PlantLabel || lplant.MetricLabelValue != plantCfg.MetricLabelValue || lplant.ThresholdPercent != plantCfg.ThresholdPercent {
				return errFencePolicyChanged
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			cooldownUntil := now.Add(time.Duration(cfg.CooldownSeconds) * time.Second).UTC()
			if _, err := e.store.UpdateRuntime(func(doc *Document) error {
				st := doc.Plants[plant]
				st.CooldownUntil = &cooldownUntil
				doc.Plants[plant] = st
				return nil
			}); err != nil {
				return fmt.Errorf("reserve cooldown: %w", err)
			}

			if e.publisher == nil {
				return fmt.Errorf("publisher unavailable")
			}
			pubCtx, cancelPublish := context.WithTimeout(ctx, time.Duration(cfg.PrometheusTimeoutSeconds)*time.Second)
			defer cancelPublish()
			var err error
			res, err = e.publisher.PublishAutomaticWater(pubCtx, plant, value, time.Duration(cfg.MessageExpirySeconds)*time.Second)
			return err
		})
		if pubErr != nil {
			if errors.Is(pubErr, errFenceRevisionChanged) || errors.Is(pubErr, errFenceDisabled) || errors.Is(pubErr, errFencePlantDisabled) || errors.Is(pubErr, errFencePolicyChanged) {
				if err := e.recordDecision(plant, DecisionDisabled, now, &sample.Timestamp, &value, sanitizeError(pubErr), ""); err != nil {
					return err
				}
				continue
			}
			err = pubErr
		}

		if err != nil {
			decision := DecisionPublishFailed
			switch {
			case errors.Is(err, messaging.ErrUnavailable), errors.Is(err, messaging.ErrDisabled), errors.Is(err, messaging.ErrTopologyNotReady), errors.Is(err, messaging.ErrDrift):
				decision = DecisionDeliveryUnavailable
			case errors.Is(err, messaging.ErrPublishUncertain):
				decision = DecisionPublishUncertain
			}
			if err := e.recordDecision(plant, decision, now, &sample.Timestamp, &value, sanitizeError(err), res.MessageID); err != nil {
				return err
			}
			continue
		}

		if err := e.recordDecision(plant, DecisionWaterPublished, now, &sample.Timestamp, &value, "", res.MessageID); err != nil {
			return err
		}
	}
	return nil
}

func buildAtomicValueTimestampExpr(selector string) string {
	valueExpr := fmt.Sprintf(`label_replace((%s), "%s", "value", "__name__", ".*")`, selector, reservedKindLabelName)
	tsExpr := fmt.Sprintf(`label_replace(timestamp((%s)), "%s", "timestamp", "__name__", ".*")`, selector, reservedKindLabelName)
	return fmt.Sprintf("(%s) or (%s)", valueExpr, tsExpr)
}

func deriveSampleAndSourceTimestamp(samples []prometheus.Sample, labelKey, labelValue string) (prometheus.Sample, time.Time, error) {
	if len(samples) == 0 {
		return prometheus.Sample{}, time.Time{}, prometheus.ErrNoSeries
	}
	var valueSample *prometheus.Sample
	var tsSample *prometheus.Sample
	for i := range samples {
		s := samples[i]
		kind, ok := s.Metric[reservedKindLabelName]
		if !ok {
			return prometheus.Sample{}, time.Time{}, errKindPairing
		}
		switch kind {
		case "value":
			if valueSample != nil {
				return prometheus.Sample{}, time.Time{}, errMultipleSeries
			}
			valueSample = &s
		case "timestamp":
			if tsSample != nil {
				return prometheus.Sample{}, time.Time{}, errMultipleSeries
			}
			tsSample = &s
		default:
			return prometheus.Sample{}, time.Time{}, errKindPairing
		}
	}
	if valueSample == nil || tsSample == nil {
		return prometheus.Sample{}, time.Time{}, errKindPairing
	}
	if err := ensureLabelMatches(valueSample.Metric, labelKey, labelValue); err != nil {
		return prometheus.Sample{}, time.Time{}, err
	}
	if err := ensureLabelMatches(tsSample.Metric, labelKey, labelValue); err != nil {
		return prometheus.Sample{}, time.Time{}, err
	}
	if !labelsEquivalentForPair(valueSample.Metric, tsSample.Metric) {
		return prometheus.Sample{}, time.Time{}, errKindPairing
	}
	sourceTS, err := unixFromSampleValue(tsSample.Value)
	if err != nil {
		return prometheus.Sample{}, time.Time{}, err
	}
	return *valueSample, sourceTS, nil
}

func labelsEquivalentForPair(valueLabels, tsLabels map[string]string) bool {
	left := map[string]string{}
	for k, v := range valueLabels {
		if k == reservedKindLabelName || k == "__name__" {
			continue
		}
		left[k] = v
	}
	right := map[string]string{}
	for k, v := range tsLabels {
		if k == reservedKindLabelName || k == "__name__" {
			continue
		}
		right[k] = v
	}
	if len(left) != len(right) {
		return false
	}
	for k, lv := range left {
		rv, ok := right[k]
		if !ok || rv != lv {
			return false
		}
	}
	return true
}

func ensureLabelMatches(metric map[string]string, labelKey, labelValue string) error {
	if metric == nil {
		return fmt.Errorf("missing labels")
	}
	v, ok := metric[labelKey]
	if !ok {
		return fmt.Errorf("missing configured label")
	}
	if v != labelValue {
		return fmt.Errorf("configured label mismatch")
	}
	return nil
}

func unixFromSampleValue(v float64) (time.Time, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return time.Time{}, fmt.Errorf("invalid source timestamp")
	}
	sec := int64(v)
	nsec := int64((v - float64(sec)) * float64(time.Second))
	ts := time.Unix(sec, nsec).UTC()
	if ts.IsZero() {
		return time.Time{}, fmt.Errorf("invalid source timestamp")
	}
	return ts, nil
}

func (e *Evaluator) recordDecision(plant string, decision Decision, evaluatedAt time.Time, sampleAt *time.Time, value *float64, errorText, messageID string) error {
	_, err := e.store.UpdateRuntime(func(doc *Document) error {
		st, ok := doc.Plants[plant]
		if !ok {
			return ErrUnknownPlant
		}
		now := evaluatedAt.UTC()
		st.LastEvaluatedAt = &now
		if sampleAt != nil {
			t := sampleAt.UTC()
			st.LastSampleAt = &t
		} else {
			st.LastSampleAt = nil
		}
		if value != nil {
			v := *value
			st.LastValue = &v
		} else {
			st.LastValue = nil
		}
		st.LastDecision = string(decision)
		st.LastError = strings.TrimSpace(errorText)
		if messageID != "" {
			st.LastMessageID = messageID
		}
		doc.Plants[plant] = st

		ageSeconds := ""
		if sampleAt != nil {
			ageSeconds = fmt.Sprintf("%.0f", evaluatedAt.Sub(*sampleAt).Seconds())
		}
		valueString := ""
		if value != nil {
			valueString = fmt.Sprintf("%.3f", *value)
		}
		log.Printf("autowatering audit: actor=system revision=%d plant=%s decision=%s value=%s age_s=%s message_id=%s outcome=%s", doc.ConfigRevision, plant, decision, valueString, ageSeconds, messageID, decision)
		return nil
	})
	return err
}

func buildSelector(metric, labelKey, labelValue string) string {
	return fmt.Sprintf(`%s{%s="%s"}`, metric, labelKey, escapeLabelValue(labelValue))
}

func escapeLabelValue(v string) string {
	replacer := strings.NewReplacer("\\", "\\\\", `"`, `\\"`)
	return replacer.Replace(v)
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	out := strings.TrimSpace(err.Error())
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
