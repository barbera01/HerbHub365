package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

const (
	chip       = "gpiochip0"
	listenAddr = ":8181"
	// Safety cap: no single pulse/cycle may run longer than this.
	maxDuration = 10 * time.Minute
)

var relays = map[string]int{
	"BASIL":   21,
	"CHILLI":  26,
	"OREGANO": 20,
}

// Relay board is active-low.
const (
	onValue  = 0
	offValue = 1
)

// ---------------------------------------------------------------------------
// Controller: owns the GPIO lines, all access serialised via mutex
// ---------------------------------------------------------------------------

// job represents a background pulse/cycle. Ownership of a channel is tracked
// by pointer identity, which (unlike comparing CancelFuncs with %p) is a
// reliable comparison.
type job struct {
	cancel context.CancelFunc
}

type controller struct {
	mu     sync.Mutex
	lines  *gpiocdev.Lines
	index  map[string]int  // channel -> offset position in request
	state  map[string]bool // channel -> currently on?
	values []int           // shadow of hardware line values, indexed by index
	jobs   map[string]*job // channel -> owning background job
	closed bool
}

func newController() (*controller, error) {
	offsets := make([]int, 0, len(relays))
	index := make(map[string]int, len(relays))
	for name, pin := range relays {
		index[name] = len(offsets)
		offsets = append(offsets, pin)
	}

	initial := make([]int, len(offsets))
	for i := range initial {
		initial[i] = offValue
	}

	lines, err := gpiocdev.RequestLines(
		chip,
		offsets,
		gpiocdev.AsOutput(initial...),
		gpiocdev.WithConsumer("relay-api"),
	)
	if err != nil {
		return nil, err
	}

	state := make(map[string]bool, len(relays))
	for name := range relays {
		state[name] = false
	}

	return &controller{
		lines:  lines,
		index:  index,
		state:  state,
		values: initial,
		jobs:   make(map[string]*job),
	}, nil
}

func (c *controller) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, j := range c.jobs {
		j.cancel()
	}
	c.jobs = make(map[string]*job)
	c.setAllLocked(false)
	c.closed = true // block any late writes from cancelled goroutines
	c.lines.Close()
}

// cancelJobLocked stops any background pulse/cycle owning this channel.
func (c *controller) cancelJobLocked(channel string) {
	if j, ok := c.jobs[channel]; ok {
		j.cancel()
		delete(c.jobs, channel)
	}
}

func (c *controller) setLocked(channel string, on bool) error {
	if c.closed {
		return errors.New("controller closed")
	}
	v := offValue
	if on {
		v = onValue
	}
	c.values[c.index[channel]] = v
	if err := c.lines.SetValues(c.values); err != nil {
		return err
	}
	c.state[channel] = on
	return nil
}

func (c *controller) setAllLocked(on bool) error {
	var firstErr error
	for name := range relays {
		if err := c.setLocked(name, on); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Set switches channels on/off immediately, cancelling any running job on them.
func (c *controller) Set(channels []string, on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range channels {
		c.cancelJobLocked(ch)
		if err := c.setLocked(ch, on); err != nil {
			return err
		}
	}
	return nil
}

// Pulse turns channels on for d, then off, in the background.
func (c *controller) Pulse(channels []string, d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel}
	for i, ch := range channels {
		c.cancelJobLocked(ch)
		if err := c.setLocked(ch, true); err != nil {
			cancel()
			// Roll back channels we already switched on for this job.
			for _, prev := range channels[:i] {
				delete(c.jobs, prev)
				c.setLocked(prev, false)
			}
			return err
		}
		c.jobs[ch] = j
	}

	go func() {
		select {
		case <-time.After(d):
		case <-ctx.Done():
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, ch := range channels {
			// Only touch channels this job still owns: another job or a
			// manual Set may have taken over since.
			if c.jobs[ch] == j {
				delete(c.jobs, ch)
				if err := c.setLocked(ch, false); err != nil {
					log.Printf("pulse: failed to switch %s off: %v", ch, err)
				}
			}
		}
	}()
	return nil
}

// Cycle rotates through all relays, each on for onDur, until total elapses.
func (c *controller) Cycle(total, onDur time.Duration) error {
	c.mu.Lock()
	// A cycle owns every channel.
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{cancel: cancel}
	for name := range relays {
		c.cancelJobLocked(name)
		c.jobs[name] = j
	}
	c.mu.Unlock()

	order := channelNames()

	go func() {
		defer func() {
			c.mu.Lock()
			for name := range relays {
				if c.jobs[name] == j {
					delete(c.jobs, name)
					if err := c.setLocked(name, false); err != nil {
						log.Printf("cycle: failed to switch %s off: %v", name, err)
					}
				}
			}
			c.mu.Unlock()
			log.Println("cycle finished")
		}()

		deadline := time.Now().Add(total)
		for time.Now().Before(deadline) {
			for _, name := range order {
				remaining := time.Until(deadline)
				if remaining <= 0 || ctx.Err() != nil {
					return
				}

				c.mu.Lock()
				// Skip channels another job has taken over.
				if c.jobs[name] != j {
					c.mu.Unlock()
					continue
				}
				if err := c.setLocked(name, true); err != nil {
					log.Printf("cycle: failed to switch %s on: %v", name, err)
					c.mu.Unlock()
					return
				}
				c.mu.Unlock()

				// Don't sleep past the deadline.
				sleep := onDur
				if remaining < sleep {
					sleep = remaining
				}
				select {
				case <-time.After(sleep):
				case <-ctx.Done():
					return
				}

				c.mu.Lock()
				if c.jobs[name] == j {
					if err := c.setLocked(name, false); err != nil {
						log.Printf("cycle: failed to switch %s off: %v", name, err)
					}
				}
				c.mu.Unlock()
			}
		}
	}()
	return nil
}

// Status returns a snapshot of relay states and active jobs.
func (c *controller) Status() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	channels := make(map[string]any, len(relays))
	for name, pin := range relays {
		_, jobActive := c.jobs[name]
		channels[name] = map[string]any{
			"pin":       pin,
			"on":        c.state[name],
			"jobActive": jobActive,
		}
	}
	return map[string]any{"chip": chip, "channels": channels}
}

func channelNames() []string {
	names := make([]string, 0, len(relays))
	for name := range relays {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic cycle order
	return names
}

// ---------------------------------------------------------------------------
// HTTP layer
// ---------------------------------------------------------------------------

type apiError struct {
	status int
	msg    string
}

func (e apiError) Error() string { return e.msg }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.status, map[string]string{"error": ae.msg})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func parseChannel(raw string) (string, error) {
	ch := strings.ToUpper(raw)
	if _, ok := relays[ch]; !ok {
		return "", apiError{http.StatusNotFound, fmt.Sprintf("unknown channel %q", raw)}
	}
	return ch, nil
}

func parseDuration(raw string) (time.Duration, error) {
	secs, err := strconv.ParseFloat(raw, 64)
	if err != nil || secs <= 0 {
		return 0, apiError{http.StatusBadRequest, "seconds must be a positive number"}
	}
	d := time.Duration(secs * float64(time.Second))
	if d > maxDuration {
		return 0, apiError{
			http.StatusBadRequest,
			fmt.Sprintf("duration exceeds safety cap of %s", maxDuration),
		}
	}
	return d, nil
}

func newServer(ctrl *controller) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ctrl.Status())
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// POST /relay/{channel}/on
	mux.HandleFunc("POST /relay/{channel}/on", func(w http.ResponseWriter, r *http.Request) {
		ch, err := parseChannel(r.PathValue("channel"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := ctrl.Set([]string{ch}, true); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channel": ch, "on": true})
	})

	// POST /relay/{channel}/off
	mux.HandleFunc("POST /relay/{channel}/off", func(w http.ResponseWriter, r *http.Request) {
		ch, err := parseChannel(r.PathValue("channel"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := ctrl.Set([]string{ch}, false); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channel": ch, "on": false})
	})

	// POST /relay/{channel}/pulse?seconds=3
	mux.HandleFunc("POST /relay/{channel}/pulse", func(w http.ResponseWriter, r *http.Request) {
		ch, err := parseChannel(r.PathValue("channel"))
		if err != nil {
			writeErr(w, err)
			return
		}
		d, err := parseDuration(r.URL.Query().Get("seconds"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := ctrl.Pulse([]string{ch}, d); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"channel": ch, "pulseSeconds": d.Seconds(),
		})
	})

	// POST /pulsecombo?channels=BASIL,CHILLI&seconds=3
	mux.HandleFunc("POST /pulsecombo", func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("channels")
		if raw == "" {
			writeErr(w, apiError{http.StatusBadRequest, "channels query param required"})
			return
		}
		var channels []string
		for _, part := range strings.Split(raw, ",") {
			ch, err := parseChannel(strings.TrimSpace(part))
			if err != nil {
				writeErr(w, err)
				return
			}
			channels = append(channels, ch)
		}
		d, err := parseDuration(r.URL.Query().Get("seconds"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := ctrl.Pulse(channels, d); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"channels": channels, "pulseSeconds": d.Seconds(),
		})
	})

	// POST /cycle?total=30&on=3
	mux.HandleFunc("POST /cycle", func(w http.ResponseWriter, r *http.Request) {
		total, err := parseDuration(r.URL.Query().Get("total"))
		if err != nil {
			writeErr(w, err)
			return
		}
		onDur, err := parseDuration(r.URL.Query().Get("on"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := ctrl.Cycle(total, onDur); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"totalSeconds": total.Seconds(), "onSeconds": onDur.Seconds(),
		})
	})

	// POST /all/on and /all/off
	mux.HandleFunc("POST /all/{action}", func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		if action != "on" && action != "off" {
			writeErr(w, apiError{http.StatusNotFound, "action must be on or off"})
			return
		}
		if err := ctrl.Set(channelNames(), action == "on"); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"all": action})
	})

	return logMiddleware(mux)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	ctrl, err := newController()
	if err != nil {
		log.Fatalf("failed to request GPIO lines: %v", err)
	}

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      newServer(ctrl),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("relay API listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown: relays off, lines released.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	ctrl.close()
	log.Println("relays off, lines released, bye")
}
