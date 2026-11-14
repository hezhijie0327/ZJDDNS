package main

import (
	"flag"
	"fmt"
	"os"
	"time"
	"zjddns/config"
	"zjddns/ddns"
	"zjddns/internal/log"
	"zjddns/providers"
)

const (
	// defaultConfigPath is the config file used when -config is not given.
	defaultConfigPath = "config.json"

	// fatalExitCode is the process exit status for startup failures
	// (config load/validate, provider init) — containers and service
	// managers must be able to detect a misconfigured run.
	fatalExitCode = 1
)

// fatalStartupError logs a startup failure and exits non-zero.
func fatalStartupError(format string, args ...any) {
	log.Errorf(format, args...)
	os.Exit(fatalExitCode)
}

func main() {
	// Custom usage text.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ZJDDNS - Dynamic DNS Update Client\n\n")
		fmt.Fprintf(os.Stderr, "Version: %s\n\n", getVersion())
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s -config <config file>     # Start with config file\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -generate-config          # Generate example config\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -version                  # Show version information\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s                            # Start with default config\n\n", os.Args[0])
	}

	// Parse the command line flags.
	configPath := flag.String("config", defaultConfigPath, "Path to config file")
	generateConfig := flag.Bool("generate-config", false, "Generate example config file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Handle the special commands.
	if *showVersion {
		fmt.Printf("%s %s\n", ProjectName, getVersion())
		return
	}

	if *generateConfig {
		printExampleConfig()
		return
	}

	versionStr := getVersion()
	if _, err := fmt.Print(banner(versionStr)); err != nil {
		// Degraded-but-continuing: the banner is cosmetic, startup proceeds.
		log.Warnf("CONFIG: Failed to write banner: %v", err)
	}

	// Load the configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalStartupError("CONFIG: Failed to load config: %v", err)
	}
	log.Infof("CONFIG: Loaded config: %s", *configPath)

	// Apply the defaults.
	cfg.SetDefaults()

	// Apply the log level (supports "debug:COMP1,COMP2" component filters).
	lvl, components := log.ParseLevelFilter(cfg.LogLevel, log.Info)
	log.SetLevelFilter(lvl, components)

	// Validate the configuration.
	if err := cfg.Validate(); err != nil {
		fatalStartupError("CONFIG: Invalid config: %v", err)
	}

	// Construct every configured provider; multiple providers may run at
	// once, each updating its own records.
	ps, err := providers.All(cfg)
	if err != nil {
		fatalStartupError("CONFIG: Failed to initialize provider: %v", err)
	}
	runner := ddns.New(ps, cfg)

	// The update interval drives the scheduling loop.
	updateInterval := cfg.Interval()

	// Each provider runs according to its own configured mode.
	runUpdate := func() {
		runner.Run()
	}

	// An interval of 0 means run once.
	if updateInterval <= 0 {
		log.Infof("DDNS: Single run")
		runUpdate()
		return
	}

	// Run periodically.
	interval := time.Duration(updateInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Infof("DDNS: Scheduler started: every %ds (Ctrl+C to stop)", updateInterval)

	// Run immediately, then on every tick.
	runUpdate()

	for range ticker.C {
		log.Debugf("DDNS: %s - Starting scheduled update...", time.Now().Format(time.DateTime))
		runUpdate()
	}
}

// printExampleConfig writes the example configuration to stdout.
func printExampleConfig() {
	data, err := config.Example()
	if err != nil {
		log.Errorf("CONFIG: Failed to generate example config: %v", err)
		return
	}

	fmt.Println(data)
}
