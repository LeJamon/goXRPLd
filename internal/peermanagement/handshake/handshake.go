// Package handshake implements the XRPL peer-to-peer handshake protocol.
// This includes TLS cookie generation, HTTP upgrade requests/responses,
// and session signature verification.
package handshake

import (
	"crypto/sha512"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/identity"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/token"
)

const (
	// ProtocolVersion is the supported protocol version string
	ProtocolVersion = "XRPL/2.2"

	// LegacyProtocolVersion is the legacy protocol version
	LegacyProtocolVersion = "RTXP/1.2"

	// HeaderUpgrade is the HTTP Upgrade header
	HeaderUpgrade = "Upgrade"
	// HeaderConnection is the HTTP Connection header
	HeaderConnection = "Connection"
	// HeaderConnectAs is the Connect-As header
	HeaderConnectAs = "Connect-As"
	// HeaderPublicKey is the Public-Key header
	HeaderPublicKey = "Public-Key"
	// HeaderSessionSignature is the Session-Signature header
	HeaderSessionSignature = "Session-Signature"
	// HeaderNetworkID is the Network-ID header
	HeaderNetworkID = "Network-ID"
	// HeaderNetworkTime is the Network-Time header
	HeaderNetworkTime = "Network-Time"
	// HeaderClosedLedger is the Closed-Ledger header
	HeaderClosedLedger = "Closed-Ledger"
	// HeaderPreviousLedger is the Previous-Ledger header
	HeaderPreviousLedger = "Previous-Ledger"
	// HeaderCrawl is the Crawl header
	HeaderCrawl = "Crawl"
	// HeaderUserAgent is the User-Agent header
	HeaderUserAgent = "User-Agent"

	// XRPLEpochOffset is the offset from Unix epoch to XRPL epoch (Jan 1, 2000)
	XRPLEpochOffset = 946684800

	// NetworkClockTolerance is the maximum allowed clock difference
	NetworkClockTolerance = 20 * time.Second
)

var (
	// ErrInvalidCookie is returned when the TLS cookie generation fails
	ErrInvalidCookie = errors.New("failed to generate TLS cookie")
	// ErrInvalidSignature is returned when session signature verification fails
	ErrInvalidSignature = errors.New("invalid session signature")
	// ErrMissingHeader is returned when a required header is missing
	ErrMissingHeader = errors.New("missing required header")
	// ErrNetworkMismatch is returned when network IDs don't match
	ErrNetworkMismatch = errors.New("network ID mismatch")
	// ErrClockSkew is returned when peer clock is too far off
	ErrClockSkew = errors.New("peer clock skew too large")
	// ErrSelfConnection is returned when connecting to self
	ErrSelfConnection = errors.New("self connection detected")
)

// Config holds configuration for the handshake
type Config struct {
	// UserAgent is the user agent string
	UserAgent string
	// NetworkID is the network identifier (0 for mainnet)
	NetworkID uint32
	// CrawlPublic determines if server info is public in crawl
	CrawlPublic bool
}

// DefaultConfig returns default handshake configuration
func DefaultConfig() Config {
	return Config{
		UserAgent:   "goXRPL/0.1.0",
		NetworkID:   0,
		CrawlPublic: false,
	}
}

// MakeSharedValue computes a shared value based on the TLS connection state.
// When there is no man in the middle, both sides will compute the same value.
// Reference: rippled Handshake.cpp makeSharedValue()
func MakeSharedValue(conn *tls.Conn) ([]byte, error) {
	// Get the connection state to access finished messages
	state := conn.ConnectionState()

	// Get local finished message
	localFinished := state.TLSUnique
	if len(localFinished) < 12 {
		return nil, fmt.Errorf("%w: local finished message too short", ErrInvalidCookie)
	}

	// Note: In Go's TLS implementation, we can't directly access
	// SSL_get_peer_finished like in OpenSSL. The TLSUnique value
	// is derived from the finished messages and provides similar
	// MITM protection. For full compatibility with rippled, we need
	// to use a custom approach.
	//
	// For now, we use the TLSUnique which is the tls-unique channel binding.
	// This is RFC 5929 compliant and provides MITM protection.

	// Hash with SHA-512
	h := sha512.New()
	h.Write(localFinished)
	hash := h.Sum(nil)

	// Return first 32 bytes (sha512Half equivalent)
	return hash[:32], nil
}

// MakeSharedValueFromFinished computes the shared value from raw finished messages.
// This matches rippled's exact algorithm: XOR SHA-512 hashes, then sha512Half.
func MakeSharedValueFromFinished(localFinished, peerFinished []byte) ([]byte, error) {
	if len(localFinished) < 12 || len(peerFinished) < 12 {
		return nil, fmt.Errorf("%w: finished message too short", ErrInvalidCookie)
	}

	// Hash local finished with SHA-512
	h1 := sha512.New()
	h1.Write(localFinished)
	cookie1 := h1.Sum(nil)

	// Hash peer finished with SHA-512
	h2 := sha512.New()
	h2.Write(peerFinished)
	cookie2 := h2.Sum(nil)

	// XOR the two 512-bit values
	result := make([]byte, 64)
	allZero := true
	for i := 0; i < 64; i++ {
		result[i] = cookie1[i] ^ cookie2[i]
		if result[i] != 0 {
			allZero = false
		}
	}

	// Don't allow zero result (both messages hash to same value)
	if allZero {
		return nil, fmt.Errorf("%w: identical finished messages", ErrInvalidCookie)
	}

	// Take sha512Half of the XOR result
	h := sha512.New()
	h.Write(result)
	finalHash := h.Sum(nil)

	return finalHash[:32], nil
}

// Request builds an HTTP upgrade request for peer connection.
// Reference: rippled Handshake.cpp makeRequest()
func Request(id *identity.Identity, sharedValue []byte, cfg Config) (*http.Request, error) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return nil, err
	}

	// Set standard HTTP headers
	req.Header.Set(HeaderUserAgent, cfg.UserAgent)
	req.Header.Set(HeaderUpgrade, ProtocolVersion+", "+LegacyProtocolVersion)
	req.Header.Set(HeaderConnection, "Upgrade")
	req.Header.Set(HeaderConnectAs, "Peer")
	req.Header.Set(HeaderCrawl, crawlValue(cfg.CrawlPublic))

	// Add handshake headers
	addHandshakeHeaders(req.Header, id, sharedValue, cfg)

	return req, nil
}

// Response builds an HTTP 101 Switching Protocols response.
// Reference: rippled Handshake.cpp makeResponse()
func Response(id *identity.Identity, sharedValue []byte, cfg Config) *http.Response {
	resp := &http.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Status:     "101 Switching Protocols",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}

	resp.Header.Set(HeaderConnection, "Upgrade")
	resp.Header.Set(HeaderUpgrade, ProtocolVersion)
	resp.Header.Set(HeaderConnectAs, "Peer")
	resp.Header.Set(HeaderCrawl, crawlValue(cfg.CrawlPublic))

	// Add handshake headers
	addHandshakeHeaders(resp.Header, id, sharedValue, cfg)

	return resp
}

// addHandshakeHeaders adds the common handshake headers to a request or response.
func addHandshakeHeaders(h http.Header, id *identity.Identity, sharedValue []byte, cfg Config) {
	// Network ID
	if cfg.NetworkID > 0 {
		h.Set(HeaderNetworkID, strconv.FormatUint(uint64(cfg.NetworkID), 10))
	}

	// Network time (XRPL epoch)
	networkTime := uint64(time.Now().Unix()) - XRPLEpochOffset
	h.Set(HeaderNetworkTime, strconv.FormatUint(networkTime, 10))

	// Public key
	h.Set(HeaderPublicKey, id.EncodedPublicKey())

	// Session signature - sign the shared value
	sig, err := id.Sign(sharedValue)
	if err == nil {
		h.Set(HeaderSessionSignature, base64.StdEncoding.EncodeToString(sig))
	}
}

// VerifyHandshake validates the handshake headers and verifies the session signature.
// Returns the peer's public key on success.
// Reference: rippled Handshake.cpp verifyHandshake()
func VerifyHandshake(headers http.Header, sharedValue []byte, localPubKey string, cfg Config) (*token.PublicKey, error) {
	// Get public key
	pubKeyStr := headers.Get(HeaderPublicKey)
	if pubKeyStr == "" {
		return nil, fmt.Errorf("%w: %s", ErrMissingHeader, HeaderPublicKey)
	}

	pubKey, err := token.ParsePublicKey(pubKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	// Check for self connection
	if pubKeyStr == localPubKey {
		return nil, ErrSelfConnection
	}

	// Verify network ID if specified
	if netIDStr := headers.Get(HeaderNetworkID); netIDStr != "" {
		netID, err := strconv.ParseUint(netIDStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid network ID: %w", err)
		}
		if cfg.NetworkID > 0 && uint32(netID) != cfg.NetworkID {
			return nil, fmt.Errorf("%w: expected %d, got %d", ErrNetworkMismatch, cfg.NetworkID, netID)
		}
	}

	// Verify network time
	if netTimeStr := headers.Get(HeaderNetworkTime); netTimeStr != "" {
		netTime, err := strconv.ParseInt(netTimeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid network time: %w", err)
		}

		// Convert to Unix time
		peerTime := time.Unix(netTime+XRPLEpochOffset, 0)
		ourTime := time.Now()

		diff := ourTime.Sub(peerTime)
		if diff < 0 {
			diff = -diff
		}
		if diff > NetworkClockTolerance {
			return nil, fmt.Errorf("%w: %v", ErrClockSkew, diff)
		}
	}

	// Verify session signature
	sigStr := headers.Get(HeaderSessionSignature)
	if sigStr == "" {
		return nil, fmt.Errorf("%w: %s", ErrMissingHeader, HeaderSessionSignature)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	if err := verifySessionSignature(pubKey, sharedValue, sigBytes); err != nil {
		return nil, err
	}

	return pubKey, nil
}

// verifySessionSignature verifies that the signature was made by the public key
// over the shared value (after SHA-512 half hashing).
func verifySessionSignature(pubKey *token.PublicKey, sharedValue, signature []byte) error {
	// Parse the DER-encoded signature
	sig, err := ecdsa.ParseDERSignature(signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

	// Hash the shared value with SHA-512 and take first 32 bytes
	h := sha512.New()
	h.Write(sharedValue)
	hash := h.Sum(nil)[:32]

	// Verify the signature
	if !sig.Verify(hash, pubKey.BtcecKey()) {
		return ErrInvalidSignature
	}

	return nil
}

// ParseProtocolVersion extracts the protocol version from the Upgrade header.
func ParseProtocolVersion(upgradeHeader string) string {
	// Handle comma-separated list
	versions := strings.Split(upgradeHeader, ",")
	for _, v := range versions {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "XRPL/") || strings.HasPrefix(v, "RTXP/") {
			return v
		}
	}
	return ""
}

// crawlValue returns the crawl header value.
func crawlValue(public bool) string {
	if public {
		return "public"
	}
	return "private"
}
