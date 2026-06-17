package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mcpwatch/internal/engine"
	"mcpwatch/internal/server"
	"mcpwatch/internal/storage"
	"mcpwatch/internal/transport"
	"mcpwatch/web"
)

var Version = "dev"

type Config struct {
	WrapCmd   string `json:"wrap"`
	ProxyURL  string `json:"proxy"`
	ProxyPort string `json:"proxy_port"`
	PID       int    `json:"pid"`
	DBPath    string `json:"db"`
	UIPort    string `json:"ui"`
	LogLevel  string `json:"log_level"`
	LogJSON   bool   `json:"log_json"`
	AuthUser  string `json:"auth_user"`
	AuthPass  string `json:"auth_pass"`
}

func main() {
	wrapCmd := flag.String("wrap", "", "Command to wrap (e.g. \"node server.js\")")
	proxyURL := flag.String("proxy", "", "Proxy to remote MCP server (e.g. \"http://localhost:3000\")")
	proxyPort := flag.String("proxy-port", "8081", "Local port to bind the proxy server to")
	pid := flag.Int("pid", 0, "Attach to existing process ID via eBPF")
	dbPath := flag.String("db", "mcpwatch_data", "Path to BadgerDB directory")
	uiPort := flag.String("ui", "8080", "Port for the UI dashboard")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logJSON := flag.Bool("log-json", false, "Output log in JSON format instead of text")
	authUser := flag.String("auth-user", "", "Username for dashboard basic auth")
	authPass := flag.String("auth-pass", "", "Password for dashboard basic auth")
	configFile := flag.String("config", "", "Path to JSON configuration file")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("mcpwatch version %s\n", Version)
		os.Exit(0)
	}

	cfg := Config{
		WrapCmd:   *wrapCmd,
		ProxyURL:  *proxyURL,
		ProxyPort: *proxyPort,
		PID:       *pid,
		DBPath:    *dbPath,
		UIPort:    *uiPort,
		LogLevel:  *logLevel,
		LogJSON:   *logJSON,
		AuthUser:  *authUser,
		AuthPass:  *authPass,
	}

	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read config file: %v\n", err)
			os.Exit(1)
		}
		var fileCfg Config
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse config file: %v\n", err)
			os.Exit(1)
		}

		explicitFlags := make(map[string]bool)
		flag.Visit(func(f *flag.Flag) {
			explicitFlags[f.Name] = true
		})

		if fileCfg.WrapCmd != "" {
			cfg.WrapCmd = fileCfg.WrapCmd
		}
		if fileCfg.ProxyURL != "" {
			cfg.ProxyURL = fileCfg.ProxyURL
		}
		if fileCfg.ProxyPort != "" {
			cfg.ProxyPort = fileCfg.ProxyPort
		}
		if fileCfg.PID != 0 {
			cfg.PID = fileCfg.PID
		}
		if fileCfg.DBPath != "" {
			cfg.DBPath = fileCfg.DBPath
		}
		if fileCfg.UIPort != "" {
			cfg.UIPort = fileCfg.UIPort
		}
		if fileCfg.LogLevel != "" {
			cfg.LogLevel = fileCfg.LogLevel
		}
		if fileCfg.LogJSON {
			cfg.LogJSON = fileCfg.LogJSON
		}
		if fileCfg.AuthUser != "" {
			cfg.AuthUser = fileCfg.AuthUser
		}
		if fileCfg.AuthPass != "" {
			cfg.AuthPass = fileCfg.AuthPass
		}

		if explicitFlags["wrap"] {
			cfg.WrapCmd = *wrapCmd
		}
		if explicitFlags["proxy"] {
			cfg.ProxyURL = *proxyURL
		}
		if explicitFlags["proxy-port"] {
			cfg.ProxyPort = *proxyPort
		}
		if explicitFlags["pid"] {
			cfg.PID = *pid
		}
		if explicitFlags["db"] {
			cfg.DBPath = *dbPath
		}
		if explicitFlags["ui"] {
			cfg.UIPort = *uiPort
		}
		if explicitFlags["log-level"] {
			cfg.LogLevel = *logLevel
		}
		if explicitFlags["log-json"] {
			cfg.LogJSON = *logJSON
		}
		if explicitFlags["auth-user"] {
			cfg.AuthUser = *authUser
		}
		if explicitFlags["auth-pass"] {
			cfg.AuthPass = *authPass
		}
	}

	// Configure structured logger
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(handler))

	modes := 0
	if cfg.WrapCmd != "" {
		modes++
	}
	if cfg.ProxyURL != "" {
		modes++
	}
	if cfg.PID != 0 {
		modes++
	}

	if modes != 1 {
		fmt.Println("Usage: mcpwatch [mode] [--db path] [--ui port]")
		fmt.Println("\nYou must specify EXACTLY ONE mode:")
		fmt.Println("  --wrap \"command args\"   Run and intercept stdio of a local command")
		fmt.Println("  --proxy \"url\"           Proxy and intercept HTTP/SSE to a remote server")
		fmt.Println("  --pid 1234              Attach and intercept via eBPF to an existing process (Linux only)")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	correlator := engine.NewCorrelator()

	hub := server.NewHub()
	srv := server.New(store, hub, web.Assets)
	if cfg.AuthUser != "" && cfg.AuthPass != "" {
		srv.SetAuth(cfg.AuthUser, cfg.AuthPass)
	}

	go func() {
		if err := srv.Start(cfg.UIPort); err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	parser := engine.NewJSONRPCParser()

	var handlerTrans transport.Handler
	if cfg.WrapCmd != "" {
		handlerTrans = transport.NewStdio(cfg.WrapCmd, parser)
	} else if cfg.ProxyURL != "" {
		handlerTrans = transport.NewProxy(cfg.ProxyURL, cfg.ProxyPort, parser)
	} else if cfg.PID != 0 {
		handlerTrans = transport.NewEBPF(cfg.PID, parser)
	}

	messages := make(chan *engine.Message, 1000)
	errChan := make(chan error, 1)

	go func() {
		slog.Info("Starting transport", "type", handlerTrans.Type())
		errChan <- handlerTrans.Start(ctx, messages)
	}()

	slog.Info("Core orchestrator running. Press Ctrl-C to stop.")
	for {
		select {
		case msg := <-messages:
			if msg == nil {
				continue
			}

			correlator.Process(msg)

			if err := store.Insert(msg); err != nil {
				slog.Error("failed to insert message", "error", err)
			}

			hub.Broadcast(msg)

		case err := <-errChan:
			if err != nil {
				slog.Error("Transport error", "error", err)
			}
			cancel()

		case <-ctx.Done():
			slog.Info("Shutting down gracefully...")
			return
		}
	}
}
