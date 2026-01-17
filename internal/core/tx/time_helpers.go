package tx

// RippleEpoch is January 1, 2000 00:00:00 UTC in Unix time
const RippleEpoch int64 = 946684800

// MaxXRP is the maximum amount of XRP that can exist (100 billion XRP in drops)
const MaxXRP uint64 = 100000000000000000

// ToRippleTime converts a Unix timestamp to Ripple time
// Ripple time is seconds since January 1, 2000 00:00:00 UTC
func ToRippleTime(unixTime int64) uint32 {
	if unixTime < RippleEpoch {
		return 0
	}
	return uint32(unixTime - RippleEpoch)
}

// FromRippleTime converts Ripple time to Unix timestamp
func FromRippleTime(rippleTime uint32) int64 {
	return int64(rippleTime) + RippleEpoch
}
