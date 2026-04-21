// Package queue provides a sequential video generation queue. Jobs are
// submitted to the video-narrator one at a time; the next job only starts
// once the previous one reaches completed or failed.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"HerbHub365/services/herbhub-manager/internal/post"
	"HerbHub365/services/herbhub-manager/internal/video"
)

type Phase string

const (
	PhasePending   Phase = "pending"
	PhaseRunning   Phase = "running"
	PhaseCompleted Phase = "completed"
	PhaseFailed    Phase = "failed"
	PhaseCancelled Phase = "cancelled"
)

// Item is one entry in the generation queue.
type Item struct {
	ID               string     `json:"id"`
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	AvatarID         string     `json:"avatar_id,omitempty"`
	ConcatEnabled    bool       `json:"concat_enabled"`
	ConcatIntro      string     `json:"concat_intro,omitempty"`
	ConcatOutro      string     `json:"concat_outro,omitempty"`
	ChromaKeyEnabled bool       `json:"chroma_key_enabled"`
	ChromaKeyBG      string     `json:"chroma_key_bg,omitempty"`
	Phase            Phase      `json:"phase"`
	JobID            string     `json:"job_id,omitempty"`
	Error            string     `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
}

// AddRequest describes a single post to add to the queue.
type AddRequest struct {
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	AvatarID         string `json:"avatar_id,omitempty"`
	ConcatEnabled    bool   `json:"concat_enabled"`
	ConcatIntro      string `json:"concat_intro,omitempty"`
	ConcatOutro      string `json:"concat_outro,omitempty"`
	ChromaKeyEnabled bool   `json:"chroma_key_enabled"`
	ChromaKeyBG      string `json:"chroma_key_bg,omitempty"`
}

// Manager holds the queue and processes items sequentially.
type Manager struct {
	mu          sync.Mutex
	items       []*Item
	videoClient *video.Client
	postsDir    string
}

func NewManager(videoClient *video.Client, postsDir string) *Manager {
	return &Manager{
		videoClient: videoClient,
		postsDir:    postsDir,
	}
}

// Add appends items to the queue and returns the created entries.
func (m *Manager) Add(reqs []AddRequest) []*Item {
	m.mu.Lock()
	defer m.mu.Unlock()

	var added []*Item
	for _, r := range reqs {
		item := &Item{
			ID:               newID(),
			Slug:             r.Slug,
			Title:            r.Title,
			AvatarID:         r.AvatarID,
			ConcatEnabled:    r.ConcatEnabled,
			ConcatIntro:      r.ConcatIntro,
			ConcatOutro:      r.ConcatOutro,
			ChromaKeyEnabled: r.ChromaKeyEnabled,
			ChromaKeyBG:      r.ChromaKeyBG,
			Phase:            PhasePending,
			CreatedAt:        time.Now().UTC(),
		}
		m.items = append(m.items, item)
		added = append(added, item)
	}
	return added
}

// Cancel marks a pending item as cancelled. Returns false if the item is not
// found or is no longer pending.
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.items {
		if item.ID == id && item.Phase == PhasePending {
			item.Phase = PhaseCancelled
			return true
		}
	}
	return false
}

// Items returns a snapshot of all queue items.
func (m *Manager) Items() []*Item {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Item, len(m.items))
	copy(out, m.items)
	return out
}

// PendingCount returns the number of pending items.
func (m *Manager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := 0
	for _, item := range m.items {
		if item.Phase == PhasePending {
			n++
		}
	}
	return n
}

// Run is a blocking loop that should be called in a goroutine. It processes
// pending items one at a time until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.mu.Lock()
		next := m.firstPending()
		if next != nil {
			now := time.Now().UTC()
			next.Phase = PhaseRunning
			next.StartedAt = &now
		}
		m.mu.Unlock()

		if next == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		log.Printf("queue: starting %s (%s)", next.Slug, next.ID)
		m.process(ctx, next)
	}
}

func (m *Manager) firstPending() *Item {
	for _, item := range m.items {
		if item.Phase == PhasePending {
			return item
		}
	}
	return nil
}

func (m *Manager) process(ctx context.Context, item *Item) {
	posts, err := post.FindAllPosts(m.postsDir)
	if err != nil {
		m.setFailed(item, "read posts: "+err.Error())
		return
	}

	var text string
	for _, p := range posts {
		if p.Slug == item.Slug {
			text = p.RawContent
			break
		}
	}
	if text == "" {
		m.setFailed(item, "post not found for slug: "+item.Slug)
		return
	}

	boolPtr := func(b bool) *bool { return &b }

	req := video.GenerateRequest{
		Slug:             item.Slug,
		Text:             text,
		AvatarID:         item.AvatarID,
		ConcatEnabled:    boolPtr(item.ConcatEnabled),
		ConcatIntro:      item.ConcatIntro,
		ConcatOutro:      item.ConcatOutro,
		ChromaKeyEnabled: boolPtr(item.ChromaKeyEnabled),
		ChromaKeyBG:      item.ChromaKeyBG,
	}

	jobID, err := m.videoClient.SubmitJob(req)
	if err != nil {
		m.setFailed(item, "submit job: "+err.Error())
		return
	}

	m.mu.Lock()
	item.JobID = jobID
	m.mu.Unlock()

	log.Printf("queue: job submitted slug=%s job_id=%s", item.Slug, jobID)

	// Poll until the narrator job finishes.
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		status, err := m.videoClient.Job(jobID)
		if err != nil {
			log.Printf("queue: poll %s: %v", jobID, err)
			continue
		}

		switch status.Phase {
		case "completed":
			m.setCompleted(item)
			log.Printf("queue: completed slug=%s", item.Slug)
			return
		case "failed":
			m.setFailed(item, status.Error)
			log.Printf("queue: failed slug=%s: %s", item.Slug, status.Error)
			return
		}
	}
}

func (m *Manager) setCompleted(item *Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	item.Phase = PhaseCompleted
	item.EndedAt = &now
}

func (m *Manager) setFailed(item *Item, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	item.Phase = PhaseFailed
	item.Error = reason
	item.EndedAt = &now
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
