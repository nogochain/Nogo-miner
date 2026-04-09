// Package config provides configuration management for NogoMiner
package config

import (
	"flag"
	"fmt"
	"os"
)

// Flags represents command-line flags
type Flags struct {
	ConfigPath string
	RPCURL     string
	WSURL      string
	Threads    int
	Address    string
	LogLevel   string
	Version    bool
	Help       bool
}

// ParseFlags parses command-line flags
func ParseFlags() (*Flags, error) {
	flags := &Flags{}

	flag.StringVar(&flags.ConfigPath, "config", "config.json", "Configuration file path")
	flag.StringVar(&flags.RPCURL, "rpc-url", "", "RPC server URL")
	flag.StringVar(&flags.WSURL, "ws-url", "", "WebSocket URL")
	flag.IntVar(&flags.Threads, "threads", 0, "Mining threads (0 = auto)")
	flag.StringVar(&flags.Address, "address", "", "Mining address")
	flag.StringVar(&flags.LogLevel, "log-level", "", "Log level (debug, info, warn, error)")
	flag.BoolVar(&flags.Version, "version", false, "Show version")
	flag.BoolVar(&flags.Help, "help", false, "Show help")
	flag.BoolVar(&flags.Help, "h", false, "Show help")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NogoMiner - NogoChain standalone miner\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -config config.json\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -rpc-url http://localhost:8080 -threads 4\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -address NOGO... -log-level debug\n", os.Args[0])
	}

	flag.Parse()

	return flags, nil
}

// ApplyToConfig applies flags to configuration
func (f *Flags) ApplyToConfig(cfg *Config) {
	if f.RPCURL != "" {
		cfg.RPC.URL = f.RPCURL
	}
	if f.WSURL != "" {
		cfg.RPC.WSURL = f.WSURL
	}
	if f.Threads > 0 {
		cfg.Miner.Threads = f.Threads
	}
	if f.Address != "" && len(cfg.Pools) > 0 {
		cfg.Pools[0].Address = f.Address
	}
	if f.LogLevel != "" {
		cfg.Logging.Level = f.LogLevel
	}
}
