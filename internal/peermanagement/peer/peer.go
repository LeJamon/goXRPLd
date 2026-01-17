// Package peer implements XRPL peer-to-peer connections.
// It handles establishing TLS connections, performing the HTTP upgrade handshake,
// and managing the peer protocol communication.
package peer

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/handshake"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/identity"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/token"
)

const (
	// DefaultPort is the default XRPL peer port
	DefaultPort = 51235

	// ConnectTimeout is the default connection timeout
	ConnectTimeout = 30 * time.Second

	// HandshakeTimeout is the timeout for the handshake process
	HandshakeTimeout = 10 * time.Second
)

var (
	// ErrConnectionFailed is returned when the connection cannot be established
	ErrConnectionFailed = errors.New("connection failed")
	// ErrHandshakeFailed is returned when the handshake fails
	ErrHandshakeFailed = errors.New("handshake failed")
	// ErrUpgradeFailed is returned when HTTP upgrade fails
	ErrUpgradeFailed = errors.New("HTTP upgrade failed")
	// ErrClosed is returned when the peer connection is closed
	ErrClosed = errors.New("connection closed")
)

// State represents the peer connection state
type State int

const (
	// StateDisconnected means the peer is not connected
	StateDisconnected State = iota
	// StateConnecting means a connection is being established
	StateConnecting
	// StateConnected means the peer is connected and authenticated
	StateConnected
	// StateClosing means the connection is being closed
	StateClosing
)

// Peer represents a connection to an XRPL peer node.
type Peer struct {
	mu sync.RWMutex

	// address is the peer's address (host:port)
	address string

	// identity is our node identity
	identity *identity.Identity

	// remotePubKey is the peer's public key after handshake
	remotePubKey *token.PublicKey

	// remoteVersion is the peer's protocol version
	remoteVersion string

	// conn is the underlying TLS connection
	conn *tls.Conn

	// state is the current connection state
	state State

	// config holds the connection configuration
	config Config

	// closeCh is closed when the peer is shutting down
	closeCh chan struct{}
}

// Config holds peer connection configuration
type Config struct {
	// Timeout is the connection timeout
	Timeout time.Duration

	// TLSConfig is the TLS configuration
	TLSConfig *tls.Config

	// HandshakeConfig is the handshake configuration
	HandshakeConfig handshake.Config
}

// DefaultConfig returns the default peer configuration
func DefaultConfig() Config {
	return Config{
		Timeout: ConnectTimeout,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true, // XRPL uses self-signed certs
			MinVersion:         tls.VersionTLS12,
		},
		HandshakeConfig: handshake.DefaultConfig(),
	}
}

// New creates a new peer with the given identity and configuration.
func New(id *identity.Identity, cfg Config) *Peer {
	return &Peer{
		identity: id,
		config:   cfg,
		state:    StateDisconnected,
		closeCh:  make(chan struct{}),
	}
}

// Connect establishes a connection to the specified peer address.
// The address should be in the format "host:port" or "host" (uses default port).
func (p *Peer) Connect(ctx context.Context, address string) error {
	p.mu.Lock()
	if p.state != StateDisconnected {
		p.mu.Unlock()
		return fmt.Errorf("peer already %v", p.state)
	}
	p.state = StateConnecting
	p.address = normalizeAddress(address)
	p.mu.Unlock()

	// Create deadline context if not already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.Timeout)
		defer cancel()
	}

	// Establish TLS connection
	conn, err := p.dialTLS(ctx)
	if err != nil {
		p.setState(StateDisconnected)
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()

	// Perform HTTP upgrade handshake
	if err := p.performHandshake(ctx); err != nil {
		p.closeConn()
		p.setState(StateDisconnected)
		return fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
	}

	p.setState(StateConnected)
	return nil
}

// dialTLS establishes a TLS connection to the peer.
func (p *Peer) dialTLS(ctx context.Context) (*tls.Conn, error) {
	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: p.config.Timeout,
	}

	// Dial TCP connection
	tcpConn, err := dialer.DialContext(ctx, "tcp", p.address)
	if err != nil {
		return nil, err
	}

	// Wrap with TLS
	tlsConfig := p.config.TLSConfig.Clone()
	if tlsConfig.ServerName == "" {
		host, _, _ := net.SplitHostPort(p.address)
		tlsConfig.ServerName = host
	}

	tlsConn := tls.Client(tcpConn, tlsConfig)

	// Perform TLS handshake
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	return tlsConn, nil
}

// performHandshake performs the HTTP upgrade handshake.
func (p *Peer) performHandshake(ctx context.Context) error {
	// Set handshake deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(HandshakeTimeout)
	}
	p.conn.SetDeadline(deadline)
	defer p.conn.SetDeadline(time.Time{}) // Clear deadline

	// Generate shared value from TLS connection
	sharedValue, err := handshake.MakeSharedValue(p.conn)
	if err != nil {
		return fmt.Errorf("failed to generate shared value: %w", err)
	}

	// Build and send HTTP upgrade request
	req, err := handshake.Request(p.identity, sharedValue, p.config.HandshakeConfig)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// Write the HTTP request
	if err := req.Write(p.conn); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read the HTTP response
	reader := bufio.NewReader(p.conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	defer resp.Body.Close()

	// Check for successful upgrade
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("%w: unexpected status %d", ErrUpgradeFailed, resp.StatusCode)
	}

	// Verify upgrade header
	upgrade := resp.Header.Get(handshake.HeaderUpgrade)
	if !strings.Contains(upgrade, "XRPL") && !strings.Contains(upgrade, "RTXP") {
		return fmt.Errorf("%w: invalid upgrade protocol: %s", ErrUpgradeFailed, upgrade)
	}

	// Verify handshake and get peer's public key
	pubKey, err := handshake.VerifyHandshake(
		resp.Header,
		sharedValue,
		p.identity.EncodedPublicKey(),
		p.config.HandshakeConfig,
	)
	if err != nil {
		return fmt.Errorf("handshake verification failed: %w", err)
	}

	p.mu.Lock()
	p.remotePubKey = pubKey
	p.remoteVersion = handshake.ParseProtocolVersion(upgrade)
	p.mu.Unlock()

	return nil
}

// Close closes the peer connection.
func (p *Peer) Close() error {
	p.mu.Lock()
	if p.state == StateDisconnected || p.state == StateClosing {
		p.mu.Unlock()
		return nil
	}
	p.state = StateClosing
	close(p.closeCh)
	p.mu.Unlock()

	err := p.closeConn()

	p.setState(StateDisconnected)
	return err
}

// closeConn closes the underlying connection.
func (p *Peer) closeConn() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}
	return nil
}

// setState updates the peer state.
func (p *Peer) setState(state State) {
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
}

// State returns the current connection state.
func (p *Peer) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// Address returns the peer's address.
func (p *Peer) Address() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.address
}

// RemotePublicKey returns the peer's public key after successful handshake.
func (p *Peer) RemotePublicKey() *token.PublicKey {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.remotePubKey
}

// RemoteVersion returns the peer's protocol version after successful handshake.
func (p *Peer) RemoteVersion() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.remoteVersion
}

// Connection returns the underlying TLS connection.
// Use with caution - direct access bypasses peer state management.
func (p *Peer) Connection() *tls.Conn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conn
}

// Read reads data from the peer connection.
func (p *Peer) Read(b []byte) (int, error) {
	p.mu.RLock()
	conn := p.conn
	state := p.state
	p.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return 0, ErrClosed
	}

	n, err := conn.Read(b)
	if err != nil && err != io.EOF {
		return n, fmt.Errorf("read error: %w", err)
	}
	return n, err
}

// Write writes data to the peer connection.
func (p *Peer) Write(b []byte) (int, error) {
	p.mu.RLock()
	conn := p.conn
	state := p.state
	p.mu.RUnlock()

	if state != StateConnected || conn == nil {
		return 0, ErrClosed
	}

	n, err := conn.Write(b)
	if err != nil {
		return n, fmt.Errorf("write error: %w", err)
	}
	return n, nil
}

// normalizeAddress ensures the address has a port.
func normalizeAddress(address string) string {
	if _, _, err := net.SplitHostPort(address); err != nil {
		// No port specified, add default
		return fmt.Sprintf("%s:%d", address, DefaultPort)
	}
	return address
}

// String returns a string representation of the State.
func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateClosing:
		return "closing"
	default:
		return "unknown"
	}
}
