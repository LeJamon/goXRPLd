// Package grpc provides gRPC server implementation for the XRPL node.
package grpc

import (
	"fmt"
	"net"
)

// ServerConfig holds configuration for the gRPC server.
type ServerConfig struct {
	// Address is the address to listen on (e.g., "127.0.0.1:50051")
	Address string

	// SecureGateway is a list of IP addresses that are allowed to connect
	// through a secure gateway (proxy). When set, the server will trust
	// X-Forwarded-For headers from these addresses.
	SecureGateway []string

	// MaxRecvMsgSize is the maximum message size in bytes the server can receive.
	// Default is 4MB if not set.
	MaxRecvMsgSize int

	// MaxSendMsgSize is the maximum message size in bytes the server can send.
	// Default is 4MB if not set.
	MaxSendMsgSize int
}

// DefaultServerConfig returns a ServerConfig with default values.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Address:        "127.0.0.1:50051",
		SecureGateway:  nil,
		MaxRecvMsgSize: 4 * 1024 * 1024, // 4MB
		MaxSendMsgSize: 4 * 1024 * 1024, // 4MB
	}
}

// Validate validates the server configuration.
func (c *ServerConfig) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("address is required")
	}

	// Validate the address format
	host, port, err := net.SplitHostPort(c.Address)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	if port == "" {
		return fmt.Errorf("port cannot be empty")
	}

	// Validate secure gateway IPs
	for _, ip := range c.SecureGateway {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("invalid secure_gateway IP: %s", ip)
		}
	}

	if c.MaxRecvMsgSize <= 0 {
		return fmt.Errorf("max_recv_msg_size must be positive")
	}

	if c.MaxSendMsgSize <= 0 {
		return fmt.Errorf("max_send_msg_size must be positive")
	}

	return nil
}

// IsSecureGateway checks if the given IP is in the secure gateway list.
func (c *ServerConfig) IsSecureGateway(ip string) bool {
	for _, gateway := range c.SecureGateway {
		if gateway == ip {
			return true
		}
	}
	return false
}
