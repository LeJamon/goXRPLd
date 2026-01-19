package metrics

import (
	"sync"
	"time"
)

const (
	// HighLatencyThreshold is the latency above which a peer is considered slow.
	HighLatencyThreshold = 500 * time.Millisecond

	// ExcellentLatency is the latency considered excellent.
	ExcellentLatency = 50 * time.Millisecond

	// GoodLatency is the latency considered good.
	GoodLatency = 150 * time.Millisecond

	// LatencyHistorySize is how many latency samples to keep.
	LatencyHistorySize = 10

	// BaseScore is the starting score for a new peer.
	BaseScore = 100

	// MaxScore is the maximum peer score.
	MaxScore = 1000

	// MinScore is the minimum peer score.
	MinScore = -100
)

// PeerScore tracks quality metrics for a peer.
type PeerScore struct {
	mu sync.RWMutex

	// Score components
	baseScore       int
	latencyBonus    int
	reliabilityMod  int
	uptimeMod       int
	behaviorMod     int

	// Latency tracking
	latencyHistory  []time.Duration
	latencyIndex    int
	averageLatency  time.Duration

	// Reliability tracking
	successfulPings int
	failedPings     int
	lastPingTime    time.Time
	lastPongTime    time.Time

	// Connection tracking
	connectedAt     time.Time
	disconnectCount int
	lastDisconnect  time.Time

	// Behavior tracking
	invalidMessages int
	goodMessages    int
}

// NewPeerScore creates a new peer score tracker.
func NewPeerScore() *PeerScore {
	return &PeerScore{
		baseScore:      BaseScore,
		latencyHistory: make([]time.Duration, LatencyHistorySize),
		connectedAt:    time.Now(),
	}
}

// Score returns the current peer score.
func (ps *PeerScore) Score() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	score := ps.baseScore + ps.latencyBonus + ps.reliabilityMod + ps.uptimeMod + ps.behaviorMod

	if score > MaxScore {
		return MaxScore
	}
	if score < MinScore {
		return MinScore
	}
	return score
}

// RecordLatency records a latency measurement.
func (ps *PeerScore) RecordLatency(latency time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Add to history
	ps.latencyHistory[ps.latencyIndex] = latency
	ps.latencyIndex = (ps.latencyIndex + 1) % LatencyHistorySize

	// Calculate average
	var total time.Duration
	count := 0
	for _, l := range ps.latencyHistory {
		if l > 0 {
			total += l
			count++
		}
	}
	if count > 0 {
		ps.averageLatency = total / time.Duration(count)
	}

	// Update latency bonus
	ps.updateLatencyBonus()
}

// updateLatencyBonus calculates the latency bonus.
// Must be called with lock held.
func (ps *PeerScore) updateLatencyBonus() {
	if ps.averageLatency <= 0 {
		ps.latencyBonus = 0
		return
	}

	switch {
	case ps.averageLatency <= ExcellentLatency:
		ps.latencyBonus = 50
	case ps.averageLatency <= GoodLatency:
		ps.latencyBonus = 25
	case ps.averageLatency <= HighLatencyThreshold:
		ps.latencyBonus = 0
	default:
		ps.latencyBonus = -25
	}
}

// RecordPingSuccess records a successful ping.
func (ps *PeerScore) RecordPingSuccess() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.successfulPings++
	ps.lastPongTime = time.Now()
	ps.updateReliabilityMod()
}

// RecordPingFailure records a failed ping.
func (ps *PeerScore) RecordPingFailure() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.failedPings++
	ps.updateReliabilityMod()
}

// updateReliabilityMod calculates the reliability modifier.
// Must be called with lock held.
func (ps *PeerScore) updateReliabilityMod() {
	total := ps.successfulPings + ps.failedPings
	if total == 0 {
		ps.reliabilityMod = 0
		return
	}

	successRate := float64(ps.successfulPings) / float64(total)
	switch {
	case successRate >= 0.99:
		ps.reliabilityMod = 50
	case successRate >= 0.95:
		ps.reliabilityMod = 25
	case successRate >= 0.90:
		ps.reliabilityMod = 0
	case successRate >= 0.80:
		ps.reliabilityMod = -25
	default:
		ps.reliabilityMod = -50
	}
}

// RecordDisconnect records a disconnection.
func (ps *PeerScore) RecordDisconnect() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.disconnectCount++
	ps.lastDisconnect = time.Now()
}

// RecordReconnect records a reconnection.
func (ps *PeerScore) RecordReconnect() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.connectedAt = time.Now()
	ps.updateUptimeMod()
}

// updateUptimeMod calculates the uptime modifier.
// Must be called with lock held.
func (ps *PeerScore) updateUptimeMod() {
	// Penalize frequent disconnects
	if ps.disconnectCount > 10 {
		ps.uptimeMod = -50
	} else if ps.disconnectCount > 5 {
		ps.uptimeMod = -25
	} else {
		ps.uptimeMod = 0
	}
}

// RecordInvalidMessage records an invalid message.
func (ps *PeerScore) RecordInvalidMessage() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.invalidMessages++
	ps.updateBehaviorMod()
}

// RecordGoodMessage records a valid, useful message.
func (ps *PeerScore) RecordGoodMessage() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.goodMessages++
	ps.updateBehaviorMod()
}

// updateBehaviorMod calculates the behavior modifier.
// Must be called with lock held.
func (ps *PeerScore) updateBehaviorMod() {
	// Heavy penalty for invalid messages
	if ps.invalidMessages > 100 {
		ps.behaviorMod = -100
	} else if ps.invalidMessages > 50 {
		ps.behaviorMod = -50
	} else if ps.invalidMessages > 10 {
		ps.behaviorMod = -25
	} else {
		// Small bonus for good messages
		bonus := ps.goodMessages / 100
		if bonus > 25 {
			bonus = 25
		}
		ps.behaviorMod = bonus
	}
}

// IsHighLatency returns true if the peer has high latency.
func (ps *PeerScore) IsHighLatency() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.averageLatency > HighLatencyThreshold
}

// AverageLatency returns the average latency.
func (ps *PeerScore) AverageLatency() time.Duration {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.averageLatency
}

// Uptime returns the current connection duration.
func (ps *PeerScore) Uptime() time.Duration {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return time.Since(ps.connectedAt)
}

// Stats returns a summary of the peer's statistics.
type ScoreStats struct {
	Score           int
	AverageLatency  time.Duration
	SuccessfulPings int
	FailedPings     int
	Uptime          time.Duration
	DisconnectCount int
	InvalidMessages int
	GoodMessages    int
}

// Stats returns the peer's statistics.
func (ps *PeerScore) Stats() *ScoreStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return &ScoreStats{
		Score:           ps.baseScore + ps.latencyBonus + ps.reliabilityMod + ps.uptimeMod + ps.behaviorMod,
		AverageLatency:  ps.averageLatency,
		SuccessfulPings: ps.successfulPings,
		FailedPings:     ps.failedPings,
		Uptime:          time.Since(ps.connectedAt),
		DisconnectCount: ps.disconnectCount,
		InvalidMessages: ps.invalidMessages,
		GoodMessages:    ps.goodMessages,
	}
}

// PeerScoreManager manages scores for multiple peers.
type PeerScoreManager struct {
	mu     sync.RWMutex
	scores map[string]*PeerScore // peer address -> score
}

// NewPeerScoreManager creates a new peer score manager.
func NewPeerScoreManager() *PeerScoreManager {
	return &PeerScoreManager{
		scores: make(map[string]*PeerScore),
	}
}

// GetScore returns the score tracker for a peer, creating one if needed.
func (psm *PeerScoreManager) GetScore(peerAddr string) *PeerScore {
	psm.mu.Lock()
	defer psm.mu.Unlock()

	score, exists := psm.scores[peerAddr]
	if !exists {
		score = NewPeerScore()
		psm.scores[peerAddr] = score
	}
	return score
}

// RemoveScore removes a score tracker for a peer.
func (psm *PeerScoreManager) RemoveScore(peerAddr string) {
	psm.mu.Lock()
	defer psm.mu.Unlock()
	delete(psm.scores, peerAddr)
}

// GetBestPeers returns peers sorted by score (best first).
func (psm *PeerScoreManager) GetBestPeers(limit int) []string {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	type peerScore struct {
		addr  string
		score int
	}

	peers := make([]peerScore, 0, len(psm.scores))
	for addr, score := range psm.scores {
		peers = append(peers, peerScore{addr, score.Score()})
	}

	// Sort by score descending
	for i := 0; i < len(peers)-1; i++ {
		for j := i + 1; j < len(peers); j++ {
			if peers[j].score > peers[i].score {
				peers[i], peers[j] = peers[j], peers[i]
			}
		}
	}

	result := make([]string, 0, limit)
	for i := 0; i < limit && i < len(peers); i++ {
		result = append(result, peers[i].addr)
	}
	return result
}

// GetWorstPeers returns peers sorted by score (worst first).
func (psm *PeerScoreManager) GetWorstPeers(limit int) []string {
	psm.mu.RLock()
	defer psm.mu.RUnlock()

	type peerScore struct {
		addr  string
		score int
	}

	peers := make([]peerScore, 0, len(psm.scores))
	for addr, score := range psm.scores {
		peers = append(peers, peerScore{addr, score.Score()})
	}

	// Sort by score ascending
	for i := 0; i < len(peers)-1; i++ {
		for j := i + 1; j < len(peers); j++ {
			if peers[j].score < peers[i].score {
				peers[i], peers[j] = peers[j], peers[i]
			}
		}
	}

	result := make([]string, 0, limit)
	for i := 0; i < limit && i < len(peers); i++ {
		result = append(result, peers[i].addr)
	}
	return result
}

// PeerCount returns the number of tracked peers.
func (psm *PeerScoreManager) PeerCount() int {
	psm.mu.RLock()
	defer psm.mu.RUnlock()
	return len(psm.scores)
}
