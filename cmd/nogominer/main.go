// Package main provides the NogoMiner application entry point
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nogochain/nogo-miner/internal/config"
	"github.com/nogochain/nogo-miner/internal/logger"
	"github.com/nogochain/nogo-miner/internal/miner"
	"github.com/nogochain/nogo-miner/internal/monitor"
	"github.com/nogochain/nogo-miner/internal/pool"
)

const (
	version = "1.0.0"
	appName = "NogoMiner"
)

func main() {
	// Parse command-line flags
	flags, err := config.ParseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Show version
	if flags.Version {
		fmt.Printf("%s v%s\n", appName, version)
		os.Exit(0)
	}

	// Show help
	if flags.Help {
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(flags.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply command-line flags
	flags.ApplyToConfig(cfg)

	// Create logger
	log, err := logger.New(logger.Config{
		Level:      cfg.Logging.Level,
		File:       cfg.Logging.File,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAgeDays: cfg.Logging.MaxAgeDays,
		Compress:   cfg.Logging.Compress,
		JSONFormat: cfg.Logging.JSONFormat,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.Infof("🚀 %s v%s starting", appName, version)
	log.Infof("Configuration loaded from: %s", flags.ConfigPath)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create monitor
	mon := monitor.NewMonitor(cfg.Monitor, log)
	mon.Start(ctx)

	// Create pool manager
	poolMgr := pool.NewManager(cfg.GetEnabledPools(), log)

	// Create miner
	minerCfg := &miner.MinerConfig{
		Threads:        cfg.Miner.Threads,
		BatchSize:      cfg.Miner.BatchSize,
		ShareDifficulty: cfg.Miner.ShareDifficulty,
	}

	miner := miner.NewMiner(minerCfg, poolMgr, mon, log)

	// Set up display function
	mon.SetDisplayFunc(func(stats *monitor.Stats) {
		displayStats(stats, miner)
	})

	// Start miner
	if err := miner.Start(); err != nil {
		log.Errorf("Failed to start miner: %v", err)
		os.Exit(1)
	}

	log.Info("✅ Mining started successfully")
	log.Infof("⛏️  Mining with %d threads", cfg.Miner.Threads)
	log.Infof("📍 Mining address: %s", poolMgr.GetAddress())

	// Set up signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Monitor mining status
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	// Update stats more frequently for better display (every 5 seconds)
	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()

	// Main loop
running:
	for {
		select {
		case sig := <-sigCh:
			log.Infof("Received signal %v, shutting down...", sig)
			break running
		case <-statsTicker.C:
			// Update monitor with current stats frequently
			if miner.IsRunning() {
				minerStats := miner.GetStats()
				mon.UpdateHashRate(minerStats.HashRate)
				mon.UpdateWorkers(minerStats.ActiveWorkers)
				mon.AddHashes(minerStats.TotalHashes)
			}
		case <-statusTicker.C:
			if !miner.IsRunning() {
				log.Error("Miner stopped unexpectedly")
				break running
			}
			// Log periodic status
			log.Infof("Miner running normally | Hashrate: %s | Workers: %d",
				formatHashRate(miner.GetStats().HashRate),
				miner.GetStats().ActiveWorkers)
		}
	}

	// Graceful shutdown
	log.Info("Shutting down miner...")
	
	if err := miner.Stop(); err != nil {
		log.Errorf("Error stopping miner: %v", err)
	}

	log.Info("👋 Miner stopped gracefully")
	log.Infof("📊 Final stats: %s", mon.GetSummary())
}

// displayStats displays mining statistics (called every 5s)
func displayStats(stats *monitor.Stats, m *miner.Miner) {
	minerStats := m.GetStats()
	
	fmt.Printf("╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  %-10s v%-5s                              ║\n", appName, version)
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Status: %-8s                                      ║\n", getStatus(minerStats.Running))
	fmt.Printf("║  Uptime: %-10s                                     ║\n", formatUptime(stats.Uptime))
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Hashrate: %-12s (Hashes: %d)               ║\n", 
		formatHashRate(stats.AvgHashRate), stats.TotalHashes)
	fmt.Printf("║  Workers: %-4d                                           ║\n", minerStats.ActiveWorkers)
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Shares: %-4d Accepted / %-4d Rejected / %-4d Invalid   ║\n",
		stats.AcceptedShares, stats.RejectedShares, stats.InvalidShares)
	fmt.Printf("║  Efficiency: %.1f%%                                        ║\n", getEfficiency(stats))
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Pool: %-20s                               ║\n", truncate(stats.PoolName, 20))
	fmt.Printf("║  Reward: %.6f NOGO (Est: %.6f NOGO)                  ║\n",
		stats.TotalReward, stats.EstimatedReward)
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n")
}

func getStatus(running bool) string {
	if running {
		return "✅ Running"
	}
	return "❌ Stopped"
}

func formatUptime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02dh%02dm%02ds", hours, minutes, seconds)
}

func formatHashRate(hashRate uint64) string {
	if hashRate >= 1_000_000 {
		return fmt.Sprintf("%.2f MH/s", float64(hashRate)/1e6)
	} else if hashRate >= 1_000 {
		return fmt.Sprintf("%.2f KH/s", float64(hashRate)/1e3)
	} else {
		return fmt.Sprintf("%d H/s", hashRate)
	}
}

func getEfficiency(stats *monitor.Stats) float64 {
	total := stats.AcceptedShares + stats.RejectedShares
	if total == 0 {
		return 100.0
	}
	return float64(stats.AcceptedShares) / float64(total) * 100.0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// init initializes the application
func init() {
	// Set up any necessary initialization
}
