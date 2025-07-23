# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goXRPLd is an idiomatic Go implementation of an XRPL (XRP Ledger) client with concurrent processing capabilities. This is NOT a direct translation of the C++ rippled implementation but rather a native Go implementation that follows Go conventions and patterns while maintaining protocol compatibility.

### Implementation Philosophy
- **Reference Implementation**: The C++ rippled implementation (located at `../rippled`) serves as the de facto specification since no formal XRPL specification exists
- **Idiomatic Go**: This implementation prioritizes Go best practices, patterns, and conventions over direct C++ code translation
- **Protocol Compatibility**: Must maintain full compatibility with the XRPL protocol and network while using Go-native approaches
- **Concurrent Design**: Leverages Go's concurrency primitives (goroutines, channels) rather than mimicking C++ threading patterns

## Common Development Commands

### Building
- `go build ./cmd/xrpld` - Build the main xrpld binary
- `go mod tidy` - Clean up and verify module dependencies

### Testing  
- `go test ./...` - Run all tests (note: some tests currently fail due to incomplete bridge.go file)
- `go test -v ./internal/codec/...` - Run codec tests with verbose output
- `go test ./internal/types/...` - Run type system tests

### Mock Generation
- `./internal/codec/binary-codec/testutil/mockgen.sh` - Generate mocks for binary codec interfaces

### Running the Server
- `./xrpld` or `go run ./cmd/xrpld` - Start the complete RPC server on port 8080
- Server provides multiple endpoints:
  - `http://localhost:8080/` - Main HTTP JSON-RPC endpoint
  - `http://localhost:8080/rpc` - Alternative HTTP JSON-RPC endpoint  
  - `ws://localhost:8080/ws` - WebSocket endpoint for subscriptions
  - `http://localhost:8080/health` - Health check endpoint

### Testing RPC Methods
- `curl -X POST http://localhost:8080/ -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"ping","id":1}'`
- `curl -X POST http://localhost:8080/ -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"server_info","id":1}'`
- WebSocket test: Connect to `ws://localhost:8080/ws` and send `{"command":"ping","id":1}`

## Architecture

### Core Components
- **HTTP JSON-RPC Server**: Complete JSON-RPC 2.0 server with 70+ XRPL methods
- **WebSocket Server**: Real-time subscription system using Gorilla WebSocket
- **Method Registry**: Dynamic method registration with role-based access control
- **Error System**: Complete rippled-compatible error codes and handling
- **Subscription System**: WebSocket subscriptions for ledger, transactions, accounts, order books
- **Storage Layer**: Multi-database architecture with specialized stores:
  - NodeStore (Pebble-based) for blockchain state
  - Relational DB (PostgreSQL) for structured queries  
  - In-memory caching layer
- **Codec System**: Binary and address encoding/decoding for XRPL data formats
- **Crypto**: ED25519 and secp256k1 implementations with XRPL-specific hashing
- **Transaction Processing**: Payment and other transaction type handling

### Key Directories
- `cmd/xrpld/` - Main application entry point
- `internal/rpc/` - Complete RPC system implementation:
  - `server.go` - HTTP JSON-RPC 2.0 server
  - `websocket.go` - WebSocket server with Gorilla WebSocket
  - `types.go` - Core RPC types and structures
  - `errors.go` - Complete rippled-compatible error system
  - `methods.go` - Method registration system
  - `*_methods.go` - Individual RPC method implementations (70+ methods)
  - `subscription_methods.go` - WebSocket subscription handling
- `internal/codec/` - Address and binary codec implementations  
- `internal/storage/` - Database backends (nodestore, relational)
- `internal/types/` - Core XRPL data types and serialization
- `internal/crypto/` - Cryptographic algorithms and utilities
- `internal/core/` - Ledger, transaction, and consensus core logic

### Dependencies
- Uses Pebble database for high-performance key-value storage
- PostgreSQL support for relational queries
- Gorilla WebSocket for WebSocket server implementation
- Mock generation with golang/mock for testing
- XRPL-go library integration for protocol compatibility

### Implementation Status
- ✅ **Complete RPC Skeleton**: All 70+ XRPL RPC methods implemented as skeletons
- ✅ **HTTP JSON-RPC 2.0 Server**: Full compliance with JSON-RPC 2.0 specification
- ✅ **WebSocket Server**: Real-time subscriptions with Gorilla WebSocket
- ✅ **Error System**: Complete rippled-compatible error codes and handling
- ✅ **Method Registry**: Dynamic registration with role-based access control
- ✅ **API Versioning**: Support for API versions 1, 2, and 3
- ✅ **CORS Support**: Cross-origin resource sharing for web clients
- 🔄 **Storage Integration**: Partial - methods have TODO placeholders for actual data
- 🔄 **Subscription Broadcasting**: Framework in place, needs integration with consensus/ledger systems
- 🔄 **Authentication**: Basic role system, needs admin detection implementation
- ❌ **Real Data**: All methods return placeholder data with detailed TODOs

### RPC Methods Implemented
**Server Info**: ping, server_info, server_state, random, server_definitions, feature, fee
**Ledger**: ledger, ledger_closed, ledger_current, ledger_data, ledger_entry, ledger_range  
**Account**: account_info, account_channels, account_currencies, account_lines, account_nfts, account_objects, account_offers, account_tx, gateway_balances, noripple_check
**Transaction**: tx, tx_history, submit, submit_multisigned, sign, sign_for, transaction_entry
**Utility**: book_offers, path_find, ripple_path_find, wallet_propose, deposit_authorized, channel_authorize, channel_verify, json
**NFT**: nft_buy_offers, nft_sell_offers, nft_history, nfts_by_issuer, nft_info
**Subscription**: subscribe, unsubscribe (WebSocket only)
**Admin**: stop, validation_create, manifest, peer_reservations_*, peers, consensus_info, validators, etc.

Each method includes comprehensive TODO comments explaining the required implementation logic.