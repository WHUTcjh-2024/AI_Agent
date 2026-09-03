package console

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const sessionTTL = 8 * time.Hour

type attempt struct {
	until time.Time
	count int
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[[32]byte]time.Time
	attempts map[string]attempt
	now      func() time.Time
}

func newSessions() *sessionStore {
	return &sessionStore{sessions: map[[32]byte]time.Time{}, attempts: map[string]attempt{}, now: time.Now}
}

func (s *sessionStore) allowLogin(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, value := range s.attempts {
		if !now.Before(value.until) {
			delete(s.attempts, key)
		}
	}
	a, ok := s.attempts[ip]
	if !ok {
		if len(s.attempts) >= 1024 {
			return false
		}
		a.until = now.Add(time.Minute)
	}
	if a.count >= 5 {
		return false
	}
	a.count++
	s.attempts[ip] = a
	return true
}

func (s *sessionStore) create() (string, time.Time, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", time.Time{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(token[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, expires := range s.sessions {
		if !now.Before(expires) {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 128 {
		return "", time.Time{}, fmt.Errorf("console session capacity reached")
	}
	expires := now.Add(sessionTTL)
	s.sessions[sha256.Sum256([]byte(raw))] = expires
	return raw, expires, nil
}

func (s *sessionStore) get(raw string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(raw))
	expires, ok := s.sessions[key]
	if ok && s.now().Before(expires) {
		return expires, true
	}
	delete(s.sessions, key)
	return time.Time{}, false
}

func (s *sessionStore) remove(raw string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sha256.Sum256([]byte(raw)))
}
