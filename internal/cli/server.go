package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/LeJamon/goXRPLd/internal/config"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/genesis"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/service"
	"github.com/LeJamon/goXRPLd/internal/rpc"
	"github.com/LeJamon/goXRPLd/internal/storage/nodestore"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb"
	"github.com/LeJamon/goXRPLd/internal/storage/relationaldb/postgres"
	"github.com/spf13/cobra"
)

var (
	// Server flags
	port       int
	bindAddr   string
	standalone bool
	dataDir    string
	pgConnStr  string
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
	rootCmd.Run = runServer

	// Server-specific flags
	serverCmd.Flags().IntVarP(&port, "port", "p", 8080, "port to listen on")
	serverCmd.Flags().StringVar(&bindAddr, "bind", "", "address to bind to (default: all interfaces)")
	serverCmd.Flags().BoolVarP(&standalone, "standalone", "a", true, "run in standalone mode (default: true)")
	serverCmd.Flags().StringVar(&dataDir, "data-dir", "", "data directory for storage (empty for in-memory only)")
	serverCmd.Flags().StringVar(&pgConnStr, "postgres", "", "PostgreSQL connection string for transaction indexing (e.g., postgres://user:pass@localhost:5432/xrpl)")
}

func runServer(cmd *cobra.Command, args []string) {
	if !quiet {
		fmt.Println("Starting goXRPLd - XRPL Node Implementation")
		fmt.Println("=========================================")
	}

	// Initialize storage if data directory is provided
	var db nodestore.Database
	if dataDir != "" {
		nodestorePath := filepath.Join(dataDir, "nodestore")
		config := nodestore.DefaultConfig()
		config.Path = nodestorePath

		backend, err := nodestore.CreateBackend("pebble", config)
		if err != nil {
			log.Fatal("Failed to create storage backend:", err)
		}

		if err := backend.Open(true); err != nil {
			log.Fatal("Failed to open storage backend:", err)
		}

		// Create database with cache (10000 entries, 10 minute TTL)
		db = nodestore.NewDatabase(backend, 10000, 10*time.Minute)

		if !quiet {
			fmt.Printf("Storage: %s\n", nodestorePath)
		}
	} else {
		if !quiet {
			fmt.Println("Storage: in-memory only (use --data-dir to persist)")
		}
	}

	// Initialize RelationalDB if PostgreSQL connection string is provided
	var repoManager relationaldb.RepositoryManager
	if pgConnStr != "" {
		pgConfig := relationaldb.NewConfig()
		pgConfig.ConnectionString = pgConnStr

		var err error
		repoManager, err = postgres.NewRepositoryManager(pgConfig)
		if err != nil {
			log.Fatal("Failed to create PostgreSQL repository manager:", err)
		}

		if err := repoManager.Open(context.Background()); err != nil {
			log.Fatal("Failed to open PostgreSQL connection:", err)
		}

		if !quiet {
			fmt.Println("PostgreSQL: connected for transaction indexing")
		}
	}

	// Load genesis configuration
	genesisFile, _ := cmd.Flags().GetString("genesis")
	var genesisConfig genesis.Config
	if genesisFile != "" {
		genesisJSON, err := config.LoadGenesisJSON(genesisFile)
		if err != nil {
			log.Fatal("Failed to load genesis file:", err)
		}
		if err := genesisJSON.Validate(); err != nil {
			log.Fatal("Invalid genesis file:", err)
		}
		genesisCfg, err := genesisJSON.ToGenesisConfig()
		if err != nil {
			log.Fatal("Failed to parse genesis configuration:", err)
		}
		// Convert config.GenesisConfig to genesis.Config
		genesisConfig = genesis.Config{
			TotalXRP:            genesisCfg.TotalXRP,
			CloseTimeResolution: genesisCfg.CloseTimeResolution,
			Fees: genesis.DefaultFees{
				BaseFee:          genesisCfg.BaseFee,
				ReserveBase:      genesisCfg.ReserveBase,
				ReserveIncrement: genesisCfg.ReserveIncrement,
			},
			Amendments:    genesisCfg.Amendments,
			UseModernFees: genesisCfg.UseModernFees,
		}
		// Convert initial accounts
		for _, acc := range genesisCfg.InitialAccounts {
			genesisConfig.InitialAccounts = append(genesisConfig.InitialAccounts, genesis.InitialAccount{
				Address:  acc.Address,
				Balance:  acc.Balance,
				Sequence: acc.Sequence,
				Flags:    acc.Flags,
			})
		}
		if !quiet {
			fmt.Printf("Genesis: loaded from %s\n", genesisFile)
		}
	} else {
		genesisConfig = genesis.DefaultConfig()
		if !quiet {
			fmt.Println("Genesis: using built-in defaults")
		}
	}

	// Initialize ledger service
	cfg := service.Config{
		Standalone:   standalone,
		NodeStore:    db,
		RelationalDB: repoManager,
	}
	if standalone {
		cfg.GenesisConfig = genesisConfig
	}

	ledgerService, err := service.New(cfg)
	if err != nil {
		log.Fatal("Failed to create ledger service:", err)
	}

	if err := ledgerService.Start(); err != nil {
		log.Fatal("Failed to start ledger service:", err)
	}

	// Wire up RPC services
	rpc.InitServices(rpc.NewLedgerServiceAdapter(ledgerService))

	if !quiet {
		if standalone {
			fmt.Println("Running in STANDALONE mode")
			genesisAddr, _ := ledgerService.GetGenesisAccount()
			fmt.Printf("  Genesis account: %s\n", genesisAddr)
			fmt.Printf("  Genesis ledger:  %d\n", ledgerService.GetValidatedLedgerIndex())
			fmt.Printf("  Open ledger:     %d\n", ledgerService.GetCurrentLedgerIndex())
			fmt.Println()
		}
	}

	// Create HTTP JSON-RPC server with 30 second timeout
	httpServer := rpc.NewServer(30 * time.Second)

	// Create WebSocket server for real-time subscriptions
	wsServer := rpc.NewWebSocketServer(30 * time.Second)
	wsServer.RegisterAllMethods()

	// Register HTTP endpoints
	http.Handle("/", httpServer)    // Main RPC endpoint
	http.Handle("/rpc", httpServer) // Alternative RPC endpoint
	http.Handle("/ws", wsServer)    // WebSocket endpoint

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

