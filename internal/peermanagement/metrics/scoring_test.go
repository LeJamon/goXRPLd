package metrics

import (
	"testing"
	"time"
)

func TestPeerScoreBasic(t *testing.T) {
	ps := NewPeerScore()

	// Initial score should be base score
	if ps.Score() != BaseScore {
		t.Errorf("Expected initial score %d, got %d", BaseScore, ps.Score())
	}
}

func TestPeerScoreLatency(t *testing.T) {
	ps := NewPeerScore()

	// Record excellent latency
	for i := 0; i < 10; i++ {
		ps.RecordLatency(30 * time.Millisecond)
	}

	if ps.AverageLatency() > ExcellentLatency {
		t.Errorf("Average latency should be excellent, got %v", ps.AverageLatency())
	}

	// Score should be above base due to latency bonus
	if ps.Score() <= BaseScore {
		t.Errorf("Score should be above base with excellent latency, got %d", ps.Score())
	}

	if ps.IsHighLatency() {
		t.Error("Should not be high latency")
	}
}

func TestPeerScoreHighLatency(t *testing.T) {
	ps := NewPeerScore()

	// Record high latency
	for i := 0; i < 10; i++ {
		ps.RecordLatency(600 * time.Millisecond)
	}

	if !ps.IsHighLatency() {
		t.Error("Should be high latency")
	}

	// Score should be below base due to latency penalty
	if ps.Score() >= BaseScore {
		t.Errorf("Score should be below base with high latency, got %d", ps.Score())
	}
}

func TestPeerScoreReliability(t *testing.T) {
	ps := NewPeerScore()

	// Record all successful pings
	for i := 0; i < 100; i++ {
		ps.RecordPingSuccess()
	}

	initialScore := ps.Score()

	// Record some failures
	ps2 := NewPeerScore()
	for i := 0; i < 80; i++ {
		ps2.RecordPingSuccess()
	}
	for i := 0; i < 20; i++ {
		ps2.RecordPingFailure()
	}

	// First peer should have higher score
	if ps.Score() <= ps2.Score() {
		t.Error("Peer with better reliability should have higher score")
	}

	_ = initialScore
}

func TestPeerScoreInvalidMessages(t *testing.T) {
	ps := NewPeerScore()

	// Record invalid messages
	for i := 0; i < 50; i++ {
		ps.RecordInvalidMessage()
	}

	// Score should be penalized
	if ps.Score() >= BaseScore {
		t.Errorf("Score should be below base with invalid messages, got %d", ps.Score())
	}
}

func TestPeerScoreStats(t *testing.T) {
	ps := NewPeerScore()

	ps.RecordLatency(50 * time.Millisecond)
	ps.RecordPingSuccess()
	ps.RecordPingFailure()
	ps.RecordGoodMessage()
	ps.RecordInvalidMessage()

	stats := ps.Stats()

	if stats.SuccessfulPings != 1 {
		t.Errorf("Expected 1 successful ping, got %d", stats.SuccessfulPings)
	}

	if stats.FailedPings != 1 {
		t.Errorf("Expected 1 failed ping, got %d", stats.FailedPings)
	}

	if stats.GoodMessages != 1 {
		t.Errorf("Expected 1 good message, got %d", stats.GoodMessages)
	}

	if stats.InvalidMessages != 1 {
		t.Errorf("Expected 1 invalid message, got %d", stats.InvalidMessages)
	}
}

func TestPeerScoreManager(t *testing.T) {
	psm := NewPeerScoreManager()

	// Get scores for different peers
	score1 := psm.GetScore("peer1")
	score2 := psm.GetScore("peer2")

	if score1 == nil || score2 == nil {
		t.Fatal("Scores should not be nil")
	}

	// Getting same peer should return same instance
	score1Again := psm.GetScore("peer1")
	if score1 != score1Again {
		t.Error("Should return same score instance for same peer")
	}

	if psm.PeerCount() != 2 {
		t.Errorf("Expected 2 peers, got %d", psm.PeerCount())
	}

	// Make peer1 better
	for i := 0; i < 10; i++ {
		score1.RecordLatency(30 * time.Millisecond)
		score1.RecordPingSuccess()
	}

	// Make peer2 worse
	for i := 0; i < 10; i++ {
		score2.RecordLatency(600 * time.Millisecond)
		score2.RecordPingFailure()
	}

	// Get best peers
	best := psm.GetBestPeers(1)
	if len(best) != 1 || best[0] != "peer1" {
		t.Errorf("Expected peer1 to be best, got %v", best)
	}

	// Get worst peers
	worst := psm.GetWorstPeers(1)
	if len(worst) != 1 || worst[0] != "peer2" {
		t.Errorf("Expected peer2 to be worst, got %v", worst)
	}

	// Remove peer
	psm.RemoveScore("peer1")
	if psm.PeerCount() != 1 {
		t.Errorf("Expected 1 peer after removal, got %d", psm.PeerCount())
	}
}
