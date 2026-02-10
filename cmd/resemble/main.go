package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/splusq/resemble/internal/cache"
	"github.com/splusq/resemble/internal/config"
	"github.com/splusq/resemble/internal/gitcache"
	"github.com/splusq/resemble/internal/proxy"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "resemble",
		Short:   "HTTP caching proxy for deterministic integration tests",
		Version: version,
	}

	rootCmd.AddCommand(
		initCmd(),
		startCmd(),
		stopCmd(),
		statusCmd(),
		clearCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	var cacheRepo string
	var listen string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize resemble with a cache repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cacheRepo == "" {
				return fmt.Errorf("--cache-repo is required")
			}

			// Clone cache repo
			cacheDir := ".resemble-cache"
			gc := gitcache.New(cacheRepo, cacheDir)
			if err := gc.Clone(); err != nil {
				return fmt.Errorf("cloning cache repo: %w", err)
			}

			// Write .testproxy.yml if it doesn't exist
			configPath := ".testproxy.yml"
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				if listen == "" {
					listen = ":9999"
				}
				content := fmt.Sprintf(`# resemble configuration
cache_repo: %s
listen: "%s"

defaults:
  ttl: 24h
  mode: auto
  ignore_headers:
    - Authorization
    - X-Request-Id
    - Date
    - User-Agent
  ignore_query: []
  strict_drift: false
`, cacheRepo, listen)

				if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
					return fmt.Errorf("writing config: %w", err)
				}
				fmt.Printf("Created %s\n", configPath)
			} else {
				fmt.Printf("%s already exists, skipping\n", configPath)
			}

			fmt.Printf("Cache repo cloned to %s\n", cacheDir)
			fmt.Println("Ready! Run 'resemble start --upstream <url>' to start the proxy.")
			return nil
		},
	}

	cmd.Flags().StringVar(&cacheRepo, "cache-repo", "", "Git URL for cache repository")
	cmd.Flags().StringVar(&listen, "listen", ":9999", "Listen address")
	return cmd
}

func startCmd() *cobra.Command {
	var (
		upstream string
		mode     string
		port     int
		cacheDir string
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the caching proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadDefault()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// CLI flags override config
			if upstream == "" && len(cfg.Upstreams) > 0 {
				upstream = cfg.Upstreams[0].Scheme + "://" + cfg.Upstreams[0].Host
			}
			if upstream == "" {
				return fmt.Errorf("--upstream is required (or configure upstreams in .testproxy.yml)")
			}

			if mode == "" {
				mode = cfg.Defaults.Mode
			}
			if mode == "" {
				mode = "auto"
			}

			listen := cfg.Listen
			if port != 0 {
				listen = fmt.Sprintf(":%d", port)
			}

			if cacheDir == "" {
				cacheDir = ".resemble-cache"
			}

			// Pull latest from git cache if it's a git repo
			if cfg.CacheRepo != "" {
				gc := gitcache.New(cfg.CacheRepo, cacheDir)
				gc.Pull()
			}

			store := cache.NewStore(cacheDir)

			keyOpts := cache.KeyOptions{
				IgnoreHeaders: cfg.Defaults.IgnoreHeaders,
				IgnoreQuery:   cfg.Defaults.IgnoreQuery,
			}

			srv := proxy.NewServer(listen, upstream, mode, cfg.Defaults.TTL, store, keyOpts)

			if err := srv.Start(); err != nil {
				return err
			}

			// Write PID file for stop command
			pidFile := filepath.Join(cacheDir, "resemble.pid")
			pidData := fmt.Sprintf(`{"pid":%d,"addr":"%s","upstream":"%s","mode":"%s","cache_dir":"%s"}`,
				os.Getpid(), srv.Addr(), upstream, mode, cacheDir)
			os.WriteFile(pidFile, []byte(pidData), 0644)

			fmt.Printf("resemble proxy running on %s -> %s (mode: %s)\n", srv.Addr(), upstream, mode)

			// Wait for interrupt
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			fmt.Println("\nShutting down...")
			srv.Stop()

			// Commit and push if git cache
			if cfg.CacheRepo != "" {
				gc := gitcache.New(cfg.CacheRepo, cacheDir)
				hasChanges, _ := gc.HasChanges()
				if hasChanges {
					fmt.Println("Committing cache changes...")
					gc.CommitAndPush("resemble: update cache")
				}
			}

			os.Remove(pidFile)
			return nil
		},
	}

	cmd.Flags().StringVar(&upstream, "upstream", "", "Upstream URL to proxy to")
	cmd.Flags().StringVar(&mode, "mode", "", "Proxy mode: auto, record, replay")
	cmd.Flags().IntVar(&port, "port", 0, "Listen port (overrides config)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory path")
	return cmd
}

func stopCmd() *cobra.Command {
	var cacheDir string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the proxy and push cache changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cacheDir == "" {
				cacheDir = ".resemble-cache"
			}

			pidFile := filepath.Join(cacheDir, "resemble.pid")
			data, err := os.ReadFile(pidFile)
			if err != nil {
				return fmt.Errorf("no running resemble instance found (no pid file at %s)", pidFile)
			}

			var pidInfo struct {
				PID      int    `json:"pid"`
				CacheDir string `json:"cache_dir"`
			}
			json.Unmarshal(data, &pidInfo)

			// Send SIGTERM to the process
			proc, err := os.FindProcess(pidInfo.PID)
			if err != nil {
				return fmt.Errorf("finding process %d: %w", pidInfo.PID, err)
			}

			if err := proc.Signal(syscall.SIGTERM); err != nil {
				log.Printf("Process may have already stopped: %v", err)
				os.Remove(pidFile)
			} else {
				fmt.Printf("Sent stop signal to resemble (pid %d)\n", pidInfo.PID)
			}

			// If we have a git cache, also push from here
			cfg, _ := config.LoadDefault()
			if cfg != nil && cfg.CacheRepo != "" {
				gc := gitcache.New(cfg.CacheRepo, cacheDir)
				hasChanges, _ := gc.HasChanges()
				if hasChanges {
					fmt.Println("Committing cache changes...")
					gc.CommitAndPush("resemble: update cache")
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory path")
	return cmd
}

func statusCmd() *cobra.Command {
	var cacheDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.LoadDefault()

			if cacheDir == "" {
				cacheDir = ".resemble-cache"
			}

			store := cache.NewStore(cacheDir)

			ttl := cfg.Defaults.TTL
			stats, err := store.Stats(ttl)
			if err != nil {
				return fmt.Errorf("getting stats: %w", err)
			}

			fmt.Printf("Cache directory: %s\n", cacheDir)
			fmt.Printf("Total entries:   %d\n", stats.TotalEntries)
			fmt.Printf("Total body size: %s\n", formatBytes(stats.TotalSize))
			fmt.Printf("Stale entries:   %d\n", stats.StaleEntries)

			if len(stats.Hosts) > 0 {
				fmt.Println("\nEntries by host:")
				for host, count := range stats.Hosts {
					fmt.Printf("  %s: %d\n", host, count)
				}
			}

			// Check if proxy is running
			pidFile := filepath.Join(cacheDir, "resemble.pid")
			if data, err := os.ReadFile(pidFile); err == nil {
				var pidInfo struct {
					PID      int    `json:"pid"`
					Addr     string `json:"addr"`
					Upstream string `json:"upstream"`
					Mode     string `json:"mode"`
				}
				json.Unmarshal(data, &pidInfo)
				fmt.Printf("\nProxy running: pid %d on %s -> %s (mode: %s)\n",
					pidInfo.PID, pidInfo.Addr, pidInfo.Upstream, pidInfo.Mode)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory path")
	return cmd
}

func clearCmd() *cobra.Command {
	var (
		cacheDir string
		all      bool
	)

	cmd := &cobra.Command{
		Use:   "clear [pattern]",
		Short: "Delete cache entries",
		Long:  "Delete cache entries matching a pattern, or all entries with --all",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cacheDir == "" {
				cacheDir = ".resemble-cache"
			}

			store := cache.NewStore(cacheDir)

			if all {
				cacheSubDir := filepath.Join(cacheDir, "cache")
				if err := os.RemoveAll(cacheSubDir); err != nil {
					return fmt.Errorf("removing cache: %w", err)
				}
				fmt.Println("Cleared all cache entries")
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("specify a pattern or use --all to clear everything")
			}

			pattern := args[0]
			count, err := store.DeleteByPattern(pattern)
			if err != nil {
				return fmt.Errorf("deleting: %w", err)
			}

			fmt.Printf("Deleted %d entries matching %q\n", count, pattern)
			return nil
		},
	}

	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory path")
	cmd.Flags().BoolVar(&all, "all", false, "Clear all cache entries")
	return cmd
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatHost(upstream string) string {
	host := upstream
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	return host
}
