package autowatering

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Manager struct {
	store      *Store
	evaluator  *Evaluator
	instanceID string

	faulted bool
	fault   string
}

type View struct {
	ConfigRevision int64                        `json:"config_revision"`
	Config         Config                       `json:"config"`
	RuntimeStatus  RuntimeStatus                `json:"runtime_status"`
	State          map[string]PlantRuntimeState `json:"state"`
}

func NewFaultedManager(message string) *Manager {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "automatic watering unavailable"
	}
	return &Manager{faulted: true, fault: msg, instanceID: detectInstanceID()}
}

func NewManager(store *Store, evaluator *Evaluator) *Manager {
	if store == nil {
		return NewFaultedManager("automatic watering store unavailable")
	}
	return &Manager{store: store, evaluator: evaluator, instanceID: detectInstanceID()}
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.faulted || m.evaluator == nil {
		return
	}
	m.evaluator.Start(ctx)
}

func (m *Manager) Wait() {
	if m == nil || m.faulted || m.evaluator == nil {
		return
	}
	m.evaluator.Wait()
}

func (m *Manager) Close() error {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.Close()
}

func (m *Manager) Unsafe() bool {
	if m == nil {
		return true
	}
	if m.faulted {
		return true
	}
	faulted, _ := m.store.Fault()
	return faulted
}

func (m *Manager) View() View {
	if m == nil {
		return View{ConfigRevision: 0, Config: DefaultConfig(), RuntimeStatus: RuntimeStatus{Running: false, Faulted: true, Fault: "automatic watering unavailable"}, State: emptyStateMap()}
	}
	if m.faulted || m.store == nil {
		return View{ConfigRevision: 0, Config: DefaultConfig(), RuntimeStatus: RuntimeStatus{Running: false, Faulted: true, Fault: m.fault, InstanceID: m.instanceID}, State: emptyStateMap()}
	}
	doc := m.store.Snapshot()
	runtime := RuntimeStatus{Running: false}
	if m.evaluator != nil {
		runtime = m.evaluator.Status()
	}
	faulted, fault := m.store.Fault()
	if faulted {
		runtime.Faulted = true
		runtime.Fault = fault
		runtime.Running = false
	}
	runtime.InstanceID = m.instanceID
	return View{ConfigRevision: doc.ConfigRevision, Config: doc.Config, RuntimeStatus: runtime, State: doc.Plants}
}

func (m *Manager) ReplaceConfig(expectedRevision int64, cfg Config, actor string, confirmEnable bool) (View, []string, error) {
	if m == nil || m.store == nil || m.faulted {
		return View{}, nil, ErrStoreFaulted
	}
	current := m.store.Snapshot()
	if !current.Config.Enabled && cfg.Enabled && !confirmEnable {
		return View{}, nil, fmt.Errorf("confirm_enable must be true when enabling automatic watering")
	}
	changed := changedSettingNames(current.Config, cfg)
	doc, err := m.store.ReplaceConfig(expectedRevision, cfg, actor)
	if err != nil {
		return View{}, nil, err
	}
	runtime := RuntimeStatus{Running: false}
	if m.evaluator != nil {
		runtime = m.evaluator.Status()
	}
	return View{ConfigRevision: doc.ConfigRevision, Config: doc.Config, RuntimeStatus: runtime, State: doc.Plants}, changed, nil
}

func changedSettingNames(oldCfg, newCfg Config) []string {
	out := []string{}
	add := func(name string) { out = append(out, name) }
	if oldCfg.Enabled != newCfg.Enabled {
		add("enabled")
	}
	if oldCfg.EvaluationIntervalSeconds != newCfg.EvaluationIntervalSeconds {
		add("evaluation_interval_seconds")
	}
	if oldCfg.CooldownSeconds != newCfg.CooldownSeconds {
		add("cooldown_seconds")
	}
	if oldCfg.MaxMetricAgeSeconds != newCfg.MaxMetricAgeSeconds {
		add("max_metric_age_seconds")
	}
	if oldCfg.PrometheusTimeoutSeconds != newCfg.PrometheusTimeoutSeconds {
		add("prometheus_timeout_seconds")
	}
	if oldCfg.MessageExpirySeconds != newCfg.MessageExpirySeconds {
		add("message_expiry_seconds")
	}
	if oldCfg.MoistureMetric != newCfg.MoistureMetric {
		add("moisture_metric")
	}
	if oldCfg.PlantLabel != newCfg.PlantLabel {
		add("plant_label")
	}
	for _, plant := range fixedPlants {
		o := oldCfg.Plants[plant]
		n := newCfg.Plants[plant]
		if o.Enabled != n.Enabled {
			add("plants." + plant + ".enabled")
		}
		if o.ThresholdPercent != n.ThresholdPercent {
			add("plants." + plant + ".threshold_percent")
		}
		if o.MetricLabelValue != n.MetricLabelValue {
			add("plants." + plant + ".metric_label_value")
		}
	}
	sort.Strings(out)
	return out
}

func ParseIfMatchRevision(raw string) (int64, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, errMissingIfMatch
	}
	m := strongETagPattern.FindStringSubmatch(v)
	if len(m) != 2 {
		return 0, fmt.Errorf("invalid If-Match revision")
	}
	rev, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || rev < 1 {
		return 0, fmt.Errorf("invalid If-Match revision")
	}
	return rev, nil
}

var errMissingIfMatch = errors.New("missing If-Match header")
var strongETagPattern = regexp.MustCompile(`^"([1-9][0-9]*)"$`)

func IsMissingIfMatch(err error) bool {
	return errors.Is(err, errMissingIfMatch)
}

func ETagForRevision(rev int64) string {
	if rev < 0 {
		rev = 0
	}
	return fmt.Sprintf(`"%d"`, rev)
}

func emptyStateMap() map[string]PlantRuntimeState {
	out := map[string]PlantRuntimeState{}
	for _, p := range fixedPlants {
		out[p] = PlantRuntimeState{}
	}
	return out
}

func nowUTC() time.Time { return time.Now().UTC() }

func detectInstanceID() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "unknown"
	}
	return host
}
