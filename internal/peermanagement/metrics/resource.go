package metrics

import (
	"sync"
	"time"
)

const (
	// DefaultChargeLimit is the default maximum resource charge.
	DefaultChargeLimit = 10000

	// DefaultDecayRate is how much charge decays per second.
	DefaultDecayRate = 100

	// DefaultWarningThreshold is when to start warning about high usage.
	DefaultWarningThreshold = 0.75

	// DefaultDisconnectThreshold is when to disconnect a peer.
	DefaultDisconnectThreshold = 1.0
)

// ChargeType represents a type of resource charge.
type ChargeType int

const (
	// ChargeNone is no charge.
	ChargeNone ChargeType = iota
	// ChargeLow is a low charge.
	ChargeLow
	// ChargeMedium is a medium charge.
	ChargeMedium
	// ChargeHigh is a high charge.
	ChargeHigh
	// ChargeInvalid is an invalid request charge.
	ChargeInvalid
)

// ChargeAmount returns the amount for a charge type.
func (c ChargeType) Amount() int {
	switch c {
	case ChargeNone:
		return 0
	case ChargeLow:
		return 10
	case ChargeMedium:
		return 50
	case ChargeHigh:
		return 200
	case ChargeInvalid:
		return 500
	default:
		return 0
	}
}

// ResourceConsumer tracks resource usage for a peer.
type ResourceConsumer struct {
	mu sync.Mutex

	// Current charge level
	charge int

	// Configuration
	limit            int
	decayRate        int
	warningThreshold float64

	// Timestamps
	lastDecay time.Time
}

// NewResourceConsumer creates a new resource consumer.
func NewResourceConsumer() *ResourceConsumer {
	return &ResourceConsumer{
		charge:           0,
		limit:            DefaultChargeLimit,
		decayRate:        DefaultDecayRate,
		warningThreshold: DefaultWarningThreshold,
		lastDecay:        time.Now(),
	}
}

// Charge adds a charge to the consumer.
// Returns true if the charge was accepted, false if limit exceeded.
func (rc *ResourceConsumer) Charge(chargeType ChargeType) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Apply decay first
	rc.applyDecay()

	amount := chargeType.Amount()
	if amount == 0 {
		return true
	}

	// Check if this would exceed the limit
	if rc.charge+amount > rc.limit {
		return false
	}

	rc.charge += amount
	return true
}

// ChargeAmount adds a specific amount to the consumer.
func (rc *ResourceConsumer) ChargeAmount(amount int) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.applyDecay()

	if rc.charge+amount > rc.limit {
		return false
	}

	rc.charge += amount
	return true
}

// Usage returns the current usage as a fraction of the limit.
func (rc *ResourceConsumer) Usage() float64 {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.applyDecay()
	return float64(rc.charge) / float64(rc.limit)
}

// IsWarning returns true if usage is above the warning threshold.
func (rc *ResourceConsumer) IsWarning() bool {
	return rc.Usage() >= rc.warningThreshold
}

// ShouldDisconnect returns true if usage is at the disconnect threshold.
func (rc *ResourceConsumer) ShouldDisconnect() bool {
	return rc.Usage() >= DefaultDisconnectThreshold
}

// Reset resets the charge to zero.
func (rc *ResourceConsumer) Reset() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.charge = 0
	rc.lastDecay = time.Now()
}

// CurrentCharge returns the current charge level.
func (rc *ResourceConsumer) CurrentCharge() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.applyDecay()
	return rc.charge
}

// applyDecay applies time-based decay to the charge.
// Must be called with the lock held.
func (rc *ResourceConsumer) applyDecay() {
	now := time.Now()
	elapsed := now.Sub(rc.lastDecay)
	rc.lastDecay = now

	decay := int(elapsed.Seconds()) * rc.decayRate
	if decay > 0 {
		rc.charge -= decay
		if rc.charge < 0 {
			rc.charge = 0
		}
	}
}

// ResourceManager manages resource consumers for multiple peers.
type ResourceManager struct {
	mu        sync.RWMutex
	consumers map[string]*ResourceConsumer // peer address -> consumer
}

// NewResourceManager creates a new resource manager.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		consumers: make(map[string]*ResourceConsumer),
	}
}

// GetConsumer returns the consumer for a peer, creating one if needed.
func (rm *ResourceManager) GetConsumer(peerAddr string) *ResourceConsumer {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	consumer, exists := rm.consumers[peerAddr]
	if !exists {
		consumer = NewResourceConsumer()
		rm.consumers[peerAddr] = consumer
	}
	return consumer
}

// RemoveConsumer removes a consumer for a peer.
func (rm *ResourceManager) RemoveConsumer(peerAddr string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.consumers, peerAddr)
}

// GetWarningPeers returns peers with high resource usage.
func (rm *ResourceManager) GetWarningPeers() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var result []string
	for addr, consumer := range rm.consumers {
		if consumer.IsWarning() {
			result = append(result, addr)
		}
	}
	return result
}

// GetOverloadedPeers returns peers that should be disconnected.
func (rm *ResourceManager) GetOverloadedPeers() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var result []string
	for addr, consumer := range rm.consumers {
		if consumer.ShouldDisconnect() {
			result = append(result, addr)
		}
	}
	return result
}

// PeerCount returns the number of tracked peers.
func (rm *ResourceManager) PeerCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.consumers)
}
