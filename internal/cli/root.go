package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	configFile string
	debug      bool
	verbose    bool
	quiet      bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "xrpld",
	Short: "goXRPLd - XRPL Node Implementation in Go",
	Long: `goXRPLd is an idiomatic Go implementation of an XRPL (XRP Ledger) client 
with concurrent processing capabilities. This is NOT a direct translation of the 
C++ rippled implementation but rather a native Go implementation that follows 
Go conventions and patterns while maintaining protocol compatibility.`,
	Version: "0.1.0-dev",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "conf", "", "configuration file path")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable normally suppressed debug logging")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress output to console after startup")
	
	// Additional flags matching rippled
	rootCmd.PersistentFlags().Bool("standalone", false, "run with no peers")
	rootCmd.PersistentFlags().Bool("silent", false, "no output to console after startup")
	rootCmd.PersistentFlags().String("genesis", "", "path to genesis JSON file (empty uses built-in defaults)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// TODO: Initialize configuration using the existing config system
	// This should integrate with internal/config package
}