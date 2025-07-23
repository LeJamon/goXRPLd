package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/LeJamon/goXRPLd/internal/rpc"
)

func main() {
	fmt.Println("Starting goXRPLd - XRPL Node Implementation")
	fmt.Println("=========================================")

	// Create HTTP JSON-RPC server with 30 second timeout
	httpServer := rpc.NewServer(30 * time.Second)
	
	// Create WebSocket server for real-time subscriptions
	wsServer := rpc.NewWebSocketServer(30 * time.Second)
	wsServer.RegisterAllMethods()

	// Register HTTP endpoints
	http.Handle("/", httpServer)           // Main RPC endpoint
	http.Handle("/rpc", httpServer)        // Alternative RPC endpoint
	http.Handle("/ws", wsServer)           // WebSocket endpoint

	// Add a simple health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"goXRPLd"}`))
	})

	fmt.Println("Server Configuration:")
	fmt.Printf("  - HTTP JSON-RPC: http://localhost:8080/\n")
	fmt.Printf("  - HTTP JSON-RPC: http://localhost:8080/rpc\n") 
	fmt.Printf("  - WebSocket:     ws://localhost:8080/ws\n")
	fmt.Printf("  - Health Check:  http://localhost:8080/health\n")
	fmt.Println()
	fmt.Println("Supported Features:")
	fmt.Printf("  - All XRPL RPC methods (%d+ methods implemented)\n", 70)
	fmt.Printf("  - WebSocket subscriptions (ledger, transactions, accounts, etc.)\n")
	fmt.Printf("  - JSON-RPC 2.0 compliance\n")
	fmt.Printf("  - CORS support\n")
	fmt.Printf("  - Error codes matching rippled\n")
	fmt.Printf("  - Multi-version API support (v1, v2, v3)\n")
	fmt.Println()
	fmt.Println("Note: This is a skeleton implementation with TODO placeholders")
	fmt.Println("Real functionality requires integration with storage and consensus systems")
	fmt.Println()
	fmt.Println("Starting server on :8080...")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
