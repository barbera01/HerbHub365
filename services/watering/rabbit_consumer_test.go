package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockPulseController struct {
	err       error
	called    int
	channels  []string
	duration  time.Duration
	ctxErrSet bool
	onPulse   func()
}

func (m *mockPulseController) PulseSync(ctx context.Context, channels []string, d time.Duration) error {
	m.called++
	m.channels = append([]string(nil), channels...)
	m.duration = d
	if ctx.Err() != nil {
		m.ctxErrSet = true
	}
	if m.onPulse != nil {
		m.onPulse()
	}
	return m.err
}

type mockAcknowledger struct {
	ackCalled    int
	rejectCalled int
	ackMultiple  []bool
	requeueVals  []bool
	ackErr       error
	rejectErr    error
	sequence     []string
	onAck        func()
	onReject     func()
}

func (m *mockAcknowledger) Ack(multiple bool) error {
	m.ackCalled++
	m.ackMultiple = append(m.ackMultiple, multiple)
	m.sequence = append(m.sequence, "ack")
	if m.onAck != nil {
		m.onAck()
	}
	return m.ackErr
}

func (m *mockAcknowledger) Reject(requeue bool) error {
	m.rejectCalled++
	m.requeueVals = append(m.requeueVals, requeue)
	m.sequence = append(m.sequence, "reject")
	if m.onReject != nil {
		m.onReject()
	}
	return m.rejectErr
}

type mockCompletionLedger struct {
	isCompleteFn   func(string) bool
	markCompleteFn func(string) error
	isCalls        []string
	markCalls      []string
	sequence       []string
	onIs           func()
	onMark         func()
}

func (m *mockCompletionLedger) IsComplete(messageID string) bool {
	m.isCalls = append(m.isCalls, messageID)
	m.sequence = append(m.sequence, "is")
	if m.onIs != nil {
		m.onIs()
	}
	if m.isCompleteFn != nil {
		return m.isCompleteFn(messageID)
	}
	return false
}

func (m *mockCompletionLedger) MarkComplete(messageID string) error {
	m.markCalls = append(m.markCalls, messageID)
	m.sequence = append(m.sequence, "mark")
	if m.onMark != nil {
		m.onMark()
	}
	if m.markCompleteFn != nil {
		return m.markCompleteFn(messageID)
	}
	return nil
}

func validWaterTS(now time.Time) time.Time {
	return now.Add(-1 * time.Minute)
}

func TestHandleWateringPayloadSkipAcksWithoutPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"chilli","action":"skip","value":81.73}`), "", time.Time{}, ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected pulse not called, got %d", ctrl.called)
	}
	if ack.ackCalled != 1 || ack.rejectCalled != 0 {
		t.Fatalf("expected ack once/reject never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
}

func TestHandleWateringPayloadRejectsMalformedOrInvalid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"plant":"chilli","action":"skip","value":`},
		{name: "trailing json", body: `{"plant":"chilli","action":"skip","value":50} null`},
		{name: "unknown plant", body: `{"plant":"mint","action":"skip","value":50}`},
		{name: "unknown action", body: `{"plant":"chilli","action":"pause","value":50}`},
		{name: "value out of range", body: `{"plant":"chilli","action":"skip","value":101}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := &mockPulseController{}
			ack := &mockAcknowledger{}

			err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(tt.body), "msg-1", validWaterTS(time.Now()), ack, &mockCompletionLedger{}, time.Now)
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if ctrl.called != 0 {
				t.Fatalf("expected no pulse call, got %d", ctrl.called)
			}
			if ack.ackCalled != 0 || ack.rejectCalled != 1 {
				t.Fatalf("expected reject once/ack never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
			}
		})
	}
}

func TestHandleWateringPayloadMissingMessageIDRejectsNoPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"basil","action":"water","value":50}`), "", validWaterTS(now), ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected no pulse call, got %d", ctrl.called)
	}
	if ack.ackCalled != 0 || ack.rejectCalled != 1 {
		t.Fatalf("expected reject once/ack never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
}

func TestHandleWateringPayloadMissingTimestampRejectsNoPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"basil","action":"water","value":50}`), "msg-1", time.Time{}, ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected no pulse call, got %d", ctrl.called)
	}
	if ack.ackCalled != 0 || ack.rejectCalled != 1 {
		t.Fatalf("expected reject once/ack never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
}

func TestHandleWateringPayloadStaleTimestampRejectsNoPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-maxWaterMessageAge).Add(-time.Second)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"basil","action":"water","value":50}`), "msg-1", stale, ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected no pulse call, got %d", ctrl.called)
	}
	if ack.rejectCalled != 1 {
		t.Fatalf("expected reject once, got %d", ack.rejectCalled)
	}
}

func TestHandleWateringPayloadFutureTimestampRejectsNoPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	future := now.Add(maxFutureSkew).Add(time.Second)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"basil","action":"water","value":50}`), "msg-1", future, ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected no pulse call, got %d", ctrl.called)
	}
	if ack.rejectCalled != 1 {
		t.Fatalf("expected reject once, got %d", ack.rejectCalled)
	}
}

func TestHandleWateringPayloadValidTimestampWaterPath(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	ledger := &mockCompletionLedger{}
	pulseDuration := 15 * time.Second
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, pulseDuration, []byte(`{"plant":"ChIlLi","action":"WaTeR","value":10}`), "msg-1", validWaterTS(now), ack, ledger, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 1 {
		t.Fatalf("expected one pulse call, got %d", ctrl.called)
	}
	if ctrl.duration != pulseDuration {
		t.Fatalf("expected pulse duration %s, got %s", pulseDuration, ctrl.duration)
	}
	if ack.ackCalled != 1 || ack.rejectCalled != 0 {
		t.Fatalf("expected ack once/reject never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
	if len(ledger.isCalls) != 1 || len(ledger.markCalls) != 1 {
		t.Fatalf("expected ledger is+mark once, got is=%#v mark=%#v", ledger.isCalls, ledger.markCalls)
	}
}

func TestHandleWateringPayloadDuplicateValidDeliveryAckNoPulse(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	ledger := &mockCompletionLedger{isCompleteFn: func(string) bool { return true }}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"basil","action":"water","value":33}`), "dup-id", validWaterTS(now), ack, ledger, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 0 {
		t.Fatalf("expected no pulse call for duplicate, got %d", ctrl.called)
	}
	if ack.ackCalled != 1 || ack.rejectCalled != 0 {
		t.Fatalf("expected ack once/reject never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
	if len(ledger.markCalls) != 0 {
		t.Fatalf("expected no mark calls for duplicate, got %#v", ledger.markCalls)
	}
}

func TestHandleWateringPayloadRejectsOnPulseFailure(t *testing.T) {
	pulseErr := errors.New("gpio failure")
	ctrl := &mockPulseController{err: pulseErr}
	ack := &mockAcknowledger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"chilli","action":"water","value":42}`), "msg-1", validWaterTS(now), ack, &mockCompletionLedger{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ctrl.called != 1 {
		t.Fatalf("expected one pulse call, got %d", ctrl.called)
	}
	if ack.ackCalled != 0 || ack.rejectCalled != 1 {
		t.Fatalf("expected reject once/ack never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
}

func TestHandleWateringPayloadWaterMarkBeforeAck(t *testing.T) {
	sequence := []string{}
	ctrl := &mockPulseController{onPulse: func() { sequence = append(sequence, "pulse") }}
	ack := &mockAcknowledger{onAck: func() { sequence = append(sequence, "ack") }}
	ledger := &mockCompletionLedger{
		onIs:   func() { sequence = append(sequence, "is") },
		onMark: func() { sequence = append(sequence, "mark") },
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"oregano","action":"water","value":12}`), "order-id", validWaterTS(now), ack, ledger, func() time.Time { return now })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	want := []string{"is", "pulse", "mark", "ack"}
	if len(sequence) != len(want) {
		t.Fatalf("expected call order %#v, got %#v", want, sequence)
	}
	for i := range want {
		if sequence[i] != want[i] {
			t.Fatalf("expected call order %#v, got %#v", want, sequence)
		}
	}
}

func TestHandleWateringPayloadPersistFailureRejectsAndReturnsFatal(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{}
	ledger := &mockCompletionLedger{markCompleteFn: func(string) error { return errors.New("disk full") }}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"oregano","action":"water","value":12}`), "persist-fail-id", validWaterTS(now), ack, ledger, func() time.Time { return now })
	if err == nil {
		t.Fatalf("expected fatal error")
	}
	if !errors.Is(err, errFatalWateringSafety) {
		t.Fatalf("expected errFatalWateringSafety, got %v", err)
	}
	if ctrl.called != 1 {
		t.Fatalf("expected pulse call, got %d", ctrl.called)
	}
	if ack.ackCalled != 0 || ack.rejectCalled != 1 {
		t.Fatalf("expected reject once/ack never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
}

func TestHandleWateringPayloadAckFailureAfterPersistReturnsError(t *testing.T) {
	ctrl := &mockPulseController{}
	ack := &mockAcknowledger{ackErr: errors.New("ack io")}
	ledger := &mockCompletionLedger{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	err := handleWateringPayload(context.Background(), ctrl, 15*time.Second, []byte(`{"plant":"oregano","action":"water","value":12}`), "ack-fail-id", validWaterTS(now), ack, ledger, func() time.Time { return now })
	if err == nil {
		t.Fatalf("expected ack error")
	}
	if errors.Is(err, errFatalWateringSafety) {
		t.Fatalf("ack failure must not be fatal safety error")
	}
	if ack.ackCalled != 1 || ack.rejectCalled != 0 {
		t.Fatalf("expected ack once/reject never, got ack=%d reject=%d", ack.ackCalled, ack.rejectCalled)
	}
	if len(ledger.markCalls) != 1 {
		t.Fatalf("expected mark complete before ack failure, got %#v", ledger.markCalls)
	}
}

func TestValidateWaterDeliveryMetadata(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		id     string
		ts     time.Time
		ok     bool
		reason string
	}{
		{name: "ok", id: "x", ts: now.Add(-time.Second), ok: true},
		{name: "missing id", id: "", ts: now, ok: false, reason: "missing_message_id"},
		{name: "missing ts", id: "x", ts: time.Time{}, ok: false, reason: "missing_timestamp"},
		{name: "stale", id: "x", ts: now.Add(-maxWaterMessageAge).Add(-time.Second), ok: false, reason: "timestamp_too_old"},
		{name: "future", id: "x", ts: now.Add(maxFutureSkew).Add(time.Second), ok: false, reason: "timestamp_too_far_in_future"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := validateWaterDeliveryMetadata(tt.id, tt.ts, now)
			if ok != tt.ok {
				t.Fatalf("expected ok=%v, got %v (reason=%s)", tt.ok, ok, reason)
			}
			if reason != tt.reason {
				t.Fatalf("expected reason=%q, got %q", tt.reason, reason)
			}
		})
	}
}

func TestRunRabbitConsumerLoopStopsOnFatalNoReconnect(t *testing.T) {
	ctx := context.Background()
	calls := 0
	consume := func(ctx context.Context, cfg rabbitConfig, ctrl pulseController, pulseDuration time.Duration, ledger completionLedger) error {
		calls++
		return errFatalWateringSafety
	}

	err := runRabbitConsumerLoop(ctx, rabbitConfig{url: "amqp://x", queue: "watering.queue"}, &mockPulseController{}, 15*time.Second, &mockCompletionLedger{}, consume)
	if !errors.Is(err, errFatalWateringSafety) {
		t.Fatalf("expected fatal safety error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one consume attempt, got %d", calls)
	}
}
