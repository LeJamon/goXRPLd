package cli

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/LeJamon/goXRPLd/internal/rpc"
	"github.com/spf13/cobra"
)

var (
	// Server flags
	port     int
	bindAddr string
)

// serverCmd represents the server command (default action)
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the XRPL daemon server",
	Long: `Start the goXRPLd server which provides:
- HTTP JSON-RPC API endpoints
- WebSocket server for real-time subscriptions  
- Health check endpoint
- All XRPL protocol methods

This is the default command when no subcommand is specified.`,
	Run: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	
	// Set server as the default command
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return serverCmd.RunE(cmd, args)
	}

	// Server-specific flags
	serverCmd.Flags().IntVarP(&port, "port", "p", 8080, "port to listen on")
	serverCmd.Flags().StringVar(&bindAddr, "bind", "", "address to bind to (default: all interfaces)")
}

func runServer(cmd *cobra.Command, args []string) {
	if !quiet {
		fmt.Println("Starting goXRPLd - XRPL Node Implementation")
		fmt.Println("=========================================")
	}

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

	if !quiet {
		listenAddr := fmt.Sprintf("%s:%d", bindAddr, port)
		if bindAddr == "" {
			listenAddr = fmt.Sprintf(":%d", port)
		}
		
		fmt.Println("Server Configuration:")
		fmt.Printf("  - HTTP JSON-RPC: http://localhost:%d/\n", port)
		fmt.Printf("  - HTTP JSON-RPC: http://localhost:%d/rpc\n", port) 
		fmt.Printf("  - WebSocket:     ws://localhost:%d/ws\n", port)
		fmt.Printf("  - Health Check:  http://localhost:%d/health\n", port)
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
		fmt.Printf("Starting server on %s...\n", listenAddr)
	}

	// Determine listen address
	listenAddr := fmt.Sprintf("%s:%d", bindAddr, port)
	if bindAddr == "" {
		listenAddr = fmt.Sprintf(":%d", port)
	}

	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}