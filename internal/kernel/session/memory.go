package session

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore keeps sessions in process memory; sessions are lost on restart.
type MemoryStore struct {
	mu   sync.RWMutex
	rows map[string]memRow
}

type memRow struct {
	p   Principal
	exp time.Time
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{rows: map[string]memRow{}} }

// Save stores p under sid until expiresAt, replacing any existing row.
func (s *MemoryStore) Save(_ context.Context, sid string, p Principal, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[sid] = memRow{p: p.withoutAuthority(), exp: expiresAt}
	return nil
}

// Load returns the live session for sid; ok is false when it is missing or expired.
func (s *MemoryStore) Load(_ context.Context, sid string) (Principal, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rows[sid]
	if !ok || !time.Now().Before(r.exp) {
		return Principal{}, false, nil
	}
	return r.p, true, nil
}

// Delete removes the session for sid.
func (s *MemoryStore) Delete(_ context.Context, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, sid)
	return nil
}

// DeleteByUser removes every session of userID.
func (s *MemoryStore) DeleteByUser(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sid, r := range s.rows {
		if r.p.UserID == userID {
			delete(s.rows, sid)
		}
	}
	return nil
}

// DeleteByUserExcept removes every session of userID but keepSID.
func (s *MemoryStore) DeleteByUserExcept(_ context.Context, userID uuid.UUID, keepSID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sid, r := range s.rows {
		if r.p.UserID == userID && sid != keepSID {
			delete(s.rows, sid)
		}
	}
	return nil
}

// DeleteExpired removes expired sessions and reports how many went.
func (s *MemoryStore) DeleteExpired(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	n := 0
	for sid, r := range s.rows {
		if !now.Before(r.exp) {
			delete(s.rows, sid)
			n++
		}
	}
	return n, nil
}
