package autowatering

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"HerbHub365/services/herbhub-manager/internal/messaging"
	"HerbHub365/services/herbhub-manager/internal/prometheus"
)

type fakeProm struct {
	queryFunc func(expr string, evaluationTime time.Time) ([]prometheus.Sample, error)
	samples   []prometheus.Sample
	err       error
	count     int
}

func (f *fakeProm) QueryVectorAt(_ context.Context, expr string, evaluationTime time.Time) ([]prometheus.Sample, error) {
	f.count++
	if f.queryFunc != nil {
		return f.queryFunc(expr, evaluationTime)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.samples, nil
}

type fakePublisher struct {
	err      error
	result   messaging.AutomaticWaterResult
	calls    int
	plants   []string
	moisture []float64
	lastExp  time.Duration
}

func (f *fakePublisher) PublishAutomaticWater(_ context.Context, plant string, moisture float64, expiry time.Duration) (messaging.AutomaticWaterResult, error) {
	f.calls++
	f.plants = append(f.plants, plant)
	f.moisture = append(f.moisture, moisture)
	f.lastExp = expiry
	if f.err != nil {
		return f.result, f.err
	}
	if f.result.MessageID == "" {
		f.result.MessageID = "msg-1"
	}
	return f.result, nil
}

func newTestStore(t *testing.T, cfg Config) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := NewStore(Bootstrap{Path: path, Config: cfg, Actor: "test"})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestEvaluatorBelowThresholdPublishesWater(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	now := time.Now().UTC()
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 20, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Add(-time.Minute).Unix()), Timestamp: now},
		}, nil
	}}
	pub := &fakePublisher{}
	st := newTestStore(t, cfg)
	ev := NewEvaluator(st, promClient, pub)
	if err := ev.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if pub.calls == 0 {
		t.Fatalf("expected publish")
	}
	if pub.lastExp != 5*time.Minute {
		t.Fatalf("unexpected expiry %v", pub.lastExp)
	}
	doc := st.Snapshot()
	if doc.Plants[PlantBasil].LastDecision != string(DecisionWaterPublished) {
		t.Fatalf("unexpected decision %q", doc.Plants[PlantBasil].LastDecision)
	}
	if promClient.count != len(fixedPlants) {
		t.Fatalf("expected one query call per plant, got %d", promClient.count)
	}
}

func TestEvaluatorEqualThresholdDoesNotWater(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	now := time.Now().UTC()
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 30, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Add(-time.Minute).Unix()), Timestamp: now},
		}, nil
	}}
	pub := &fakePublisher{}
	st := newTestStore(t, cfg)
	ev := NewEvaluator(st, promClient, pub)
	if err := ev.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish")
	}
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionHealthy) {
		t.Fatalf("expected healthy decision")
	}
}

func TestEvaluatorNoMetricMultipleStaleFutureInvalid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)

	ev := NewEvaluator(st, &fakeProm{err: prometheus.ErrNoSeries}, &fakePublisher{})
	_ = ev.EvaluateOnce(context.Background())
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionNoMetric) {
		t.Fatalf("expected no_metric")
	}

	now := time.Now().UTC()
	ev = NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 11, Timestamp: now},
		}, nil
	}}, &fakePublisher{})
	_ = ev.EvaluateOnce(context.Background())
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionMultipleMetrics) {
		t.Fatalf("expected multiple_metrics")
	}

	ev = NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Add(-20 * time.Minute).Unix()), Timestamp: now},
		}, nil
	}}, &fakePublisher{})
	_ = ev.EvaluateOnce(context.Background())
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionStaleMetric) {
		t.Fatalf("expected stale_metric")
	}

	ev = NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Add(2 * time.Minute).Unix()), Timestamp: now},
		}, nil
	}}, &fakePublisher{})
	_ = ev.EvaluateOnce(context.Background())
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionFutureMetric) {
		t.Fatalf("expected future_metric")
	}

	ev = NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 101, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}, &fakePublisher{})
	_ = ev.EvaluateOnce(context.Background())
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionInvalidMetric) {
		t.Fatalf("expected invalid_metric")
	}
}

func TestEvaluatorCooldownAndPublishFailurePaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	now := time.Now().UTC()
	st := newTestStore(t, cfg)

	pub := &fakePublisher{result: messaging.AutomaticWaterResult{MessageID: "maybe-id"}, err: messaging.ErrPublishUncertain}
	ev := NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}, pub)
	if err := ev.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionPublishUncertain) {
		t.Fatalf("expected publish_uncertain")
	}

	pub2 := &fakePublisher{err: messaging.ErrUnavailable}
	ev2 := NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}, pub2)
	if err := ev2.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionCooldown) {
		t.Fatalf("expected cooldown retained after uncertain publish")
	}
}

func TestEvaluatorCancellationStopsBeforePublish(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)
	pub := &fakePublisher{err: errors.New("should not be called")}
	now := time.Now().UTC()
	ev := NewEvaluator(st, &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}, pub)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ev.EvaluateOnce(ctx); err == nil {
		t.Fatalf("expected cancellation error")
	}
	if pub.calls != 0 {
		t.Fatalf("publish should not occur")
	}
}

func TestEvaluatorSourceTimestampOldAtCurrentEvaluationBlockedStale(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)
	now := time.Now().UTC()
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Add(-2 * time.Hour).Unix()), Timestamp: now},
		}, nil
	}}
	ev := NewEvaluator(st, promClient, &fakePublisher{})
	if err := ev.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionStaleMetric) {
		t.Fatalf("expected stale decision")
	}
}

func TestEvaluatorRejectsConfiguredLabelMismatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)
	now := time.Now().UTC()
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "other", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "other", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}
	ev := NewEvaluator(st, promClient, &fakePublisher{})
	if err := ev.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if st.Snapshot().Plants[PlantBasil].LastDecision != string(DecisionInvalidMetric) {
		t.Fatalf("expected invalid metric for label mismatch")
	}
}

func TestEvaluatorFenceDisableBlocksPublish(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)
	now := time.Now().UTC()
	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		if !strings.Contains(expr, `"`+reservedKindLabelName+`"`) {
			t.Fatalf("expected reserved kind label in expression: %s", expr)
		}
		select {
		case queryStarted <- struct{}{}:
		default:
		}
		<-releaseQuery
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}
	pub := &fakePublisher{}
	ev := NewEvaluator(st, promClient, pub)
	done := make(chan error, 1)
	go func() { done <- ev.EvaluateOnce(context.Background()) }()
	<-queryStarted
	current := st.Snapshot()
	newCfg := current.Config
	newCfg.Enabled = false
	_, err := st.ReplaceConfig(current.ConfigRevision, newCfg, "tester")
	if err != nil {
		t.Fatalf("replace config: %v", err)
	}
	close(releaseQuery)
	if err := <-done; err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if pub.calls != 0 {
		t.Fatalf("publish should have been fenced off")
	}
}

func TestEvaluatorFenceHoldsAcrossPublishThenDisableWaits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	st := newTestStore(t, cfg)
	now := time.Now().UTC()
	releasePublish := make(chan struct{})
	promClient := &fakeProm{queryFunc: func(expr string, _ time.Time) ([]prometheus.Sample, error) {
		_ = expr
		return []prometheus.Sample{
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 10, Timestamp: now},
			{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
		}, nil
	}}

	startedPublish := make(chan struct{}, 1)
	blockingPublisher := &fakePublisherWithBlock{started: startedPublish, release: releasePublish, result: messaging.AutomaticWaterResult{MessageID: "msg-fenced"}}
	ev := NewEvaluator(st, promClient, blockingPublisher)

	evalDone := make(chan error, 1)
	go func() { evalDone <- ev.EvaluateOnce(context.Background()) }()
	<-startedPublish

	replaceDone := make(chan error, 1)
	go func() {
		current := st.Snapshot()
		newCfg := current.Config
		newCfg.Enabled = false
		_, err := st.ReplaceConfig(current.ConfigRevision, newCfg, "tester")
		replaceDone <- err
	}()

	select {
	case <-replaceDone:
		t.Fatalf("replace should wait while publish fence held")
	case <-time.After(75 * time.Millisecond):
	}
	close(releasePublish)
	if err := <-evalDone; err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if err := <-replaceDone; err != nil {
		t.Fatalf("replace after publish: %v", err)
	}
}

type fakePublisherWithBlock struct {
	started chan struct{}
	release chan struct{}
	result  messaging.AutomaticWaterResult
}

func TestAtomicExpressionBuilder(t *testing.T) {
	sel := `herbhub_soil_percent{plant="basil"}`
	expr := buildAtomicValueTimestampExpr(sel)
	if !strings.Contains(expr, "label_replace") || !strings.Contains(expr, "timestamp(") || !strings.Contains(expr, reservedKindLabelName) {
		t.Fatalf("unexpected atomic expression: %s", expr)
	}
}

func TestDeriveSampleAndSourceTimestampRejectsMissingKinds(t *testing.T) {
	now := time.Now().UTC()
	_, _, err := deriveSampleAndSourceTimestamp([]prometheus.Sample{{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 1, Timestamp: now}}, "plant", "basil")
	if err == nil {
		t.Fatalf("expected missing kind pair error")
	}
}

func TestDeriveSampleAndSourceTimestampRejectsDuplicateKinds(t *testing.T) {
	now := time.Now().UTC()
	_, _, err := deriveSampleAndSourceTimestamp([]prometheus.Sample{
		{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 1, Timestamp: now},
		{Metric: map[string]string{"plant": "basil", reservedKindLabelName: "value"}, Value: 2, Timestamp: now},
	}, "plant", "basil")
	if err == nil {
		t.Fatalf("expected duplicate kind error")
	}
}

func TestDeriveSampleAndSourceTimestampRejectsExtraLabelMismatch(t *testing.T) {
	now := time.Now().UTC()
	_, _, err := deriveSampleAndSourceTimestamp([]prometheus.Sample{
		{Metric: map[string]string{"plant": "basil", "zone": "left", reservedKindLabelName: "value"}, Value: 1, Timestamp: now},
		{Metric: map[string]string{"plant": "basil", "zone": "right", reservedKindLabelName: "timestamp"}, Value: float64(now.Unix()), Timestamp: now},
	}, "plant", "basil")
	if err == nil {
		t.Fatalf("expected label mismatch error")
	}
}

func (f *fakePublisherWithBlock) PublishAutomaticWater(_ context.Context, _ string, _ float64, _ time.Duration) (messaging.AutomaticWaterResult, error) {
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	<-f.release
	if f.result.MessageID == "" {
		f.result.MessageID = "msg"
	}
	return f.result, nil
}
