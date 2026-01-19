package relay

import (
	"sync"
	"time"
)

// Squelch maintains the list of validators whose messages should not be relayed.
// This is the peer-side implementation that responds to TMSquelch messages.
type Squelch struct {
	mu        sync.RWMutex
	squelched map[string]time.Time // validator public key -> expiration time
}

// NewSquelch creates a new Squelch instance.
func NewSquelch() *Squelch {
	return &Squelch{
		squelched: make(map[string]time.Time),
	}
}

// AddSquelch adds a squelch for a validator.
// Returns false if the duration is invalid.
func (s *Squelch) AddSquelch(validator []byte, duration time.Duration) bool {
	if duration < MinUnsquelchExpire || duration > MaxUnsquelchExpirePeers {
		// Invalid duration - remove any existing squelch
		s.RemoveSquelch(validator)
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.squelched[string(validator)] = time.Now().Add(duration)
	return true
}

// RemoveSquelch removes the squelch for a validator.
func (s *Squelch) RemoveSquelch(validator []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.squelched, string(validator))
}

// ExpireSquelch checks if a squelch has expired and removes it.
// Returns true if the squelch was removed or doesn't exist.
// Returns false if the squelch is still active.
func (s *Squelch) ExpireSquelch(validator []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(validator)
	expiration, exists := s.squelched[key]
	if !exists {
		return true
	}

	if time.Now().After(expiration) {
		delete(s.squelched, key)
		return true
	}

	return false
}

// IsSquelched returns true if messages from the validator should not be relayed.
func (s *Squelch) IsSquelched(validator []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := string(validator)
	expiration, exists := s.squelched[key]
	if !exists {
		return false
	}

	return time.Now().Before(expiration)
}

// GetSquelchedCount returns the number of squelched validators.
func (s *Squelch) GetSquelchedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.squelched)
}

// GetSquelchedValidators returns the list of currently squelched validators.
func (s *Squelch) GetSquelchedValidators() [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var result [][]byte
	for key, expiration := range s.squelched {
		if now.Before(expiration) {
			result = append(result, []byte(key))
		}
	}
	return result
}

// ExpireAll removes all expired squelches.
func (s *Squelch) ExpireAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, expiration := range s.squelched {
		if now.After(expiration) {
			delete(s.squelched, key)
		}
	}
}

// Clear removes all squelches.
func (s *Squelch) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.squelched = make(map[string]time.Time)
}

// GetExpiration returns the expiration time for a squelched validator.
// Returns zero time if not squelched.
func (s *Squelch) GetExpiration(validator []byte) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expiration, exists := s.squelched[string(validator)]
	if !exists {
		return time.Time{}
	}
	return expiration
}

// TimeRemaining returns the time remaining on a squelch.
// Returns 0 if not squelched or expired.
func (s *Squelch) TimeRemaining(validator []byte) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expiration, exists := s.squelched[string(validator)]
	if !exists {
		return 0
	}

	remaining := time.Until(expiration)
	if remaining < 0 {
		return 0
	}
	return remaining
}
