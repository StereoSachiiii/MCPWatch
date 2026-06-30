package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"mcpwatch/internal/alert"
	"mcpwatch/internal/engine"
	"mcpwatch/internal/metrics"
	"mcpwatch/internal/server"
	"mcpwatch/internal/storage"
	"mcpwatch/internal/tracing"
	"mcpwatch/internal/transport"
	"mcpwatch/web"
)

var Version = "dev"

type Config struct {
	WrapCmd        string  `json:"wrap"`
	ProxyURL       string  `json:"proxy"`
	ProxyPort      string  `json:"proxy_port"`
	PID            int     `json:"pid"`
	DBPath         string  `json:"db"`
	UIPort         string  `json:"ui"`
	LogLevel       string  `json:"log_level"`
	LogJSON        bool    `json:"log_json"`
	AuthUser       string  `json:"auth_user"`
	AuthPass       string  `json:"auth_pass"`
	TLSCert        string  `json:"tls_cert"`
	TLSKey         string  `json:"tls_key"`
	AlertWebhook   string  `json:"alert_webhook"`
	AlertErrorRate float64 `json:"alert_error_rate"`
	AlertLatency   int64   `json:"alert_latency"`
	AlertWindow    int     `json:"alert_window"`
	GCPercent      int     `json:"gc_percent"`
	SocketLocal    string  `json:"socket_local"`
	SocketTarget   string  `json:"socket_target"`
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
	tlsCert := flag.String("tls-cert", "", "Path to TLS certificate file for HTTPS")
	tlsKey := flag.String("tls-key", "", "Path to TLS key file for HTTPS")
	alertWebhook := flag.String("alert-webhook", "", "Webhook URL for Slack or custom alerts")
	alertErrorRate := flag.Float64("alert-error-rate", 0.0, "Error rate threshold percentage (e.g. 10.0 for 10%)")
	alertLatency := flag.Int64("alert-latency", 0, "Latency threshold in milliseconds (e.g. 5000)")
	alertWindow := flag.Int("alert-window", 60, "Check window size in seconds")
	gcPercent := flag.Int("gc-percent", 35, "Target GC garbage collection heap growth percentage (30-40 recommended)")
	socketLocal := flag.String("socket-local", "", "Local Unix socket path to listen on")
	socketTarget := flag.String("socket-target", "", "Target Unix socket path to forward to")
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
		TLSCert:   *tlsCert,
		TLSKey:    *tlsKey,
		AlertWebhook:   *alertWebhook,
		AlertErrorRate: *alertErrorRate,
		AlertLatency:   *alertLatency,
		AlertWindow:    *alertWindow,
		GCPercent:      *gcPercent,
		SocketLocal:    *socketLocal,
		SocketTarget:   *socketTarget,
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
		if fileCfg.TLSCert != "" {
			cfg.TLSCert = fileCfg.TLSCert
		}
		if fileCfg.TLSKey != "" {
			cfg.TLSKey = fileCfg.TLSKey
		}
		if fileCfg.AlertWebhook != "" {
			cfg.AlertWebhook = fileCfg.AlertWebhook
		}
		if fileCfg.AlertErrorRate > 0 {
			cfg.AlertErrorRate = fileCfg.AlertErrorRate
		}
		if fileCfg.AlertLatency > 0 {
			cfg.AlertLatency = fileCfg.AlertLatency
		}
		if fileCfg.AlertWindow > 0 {
			cfg.AlertWindow = fileCfg.AlertWindow
		}
		if fileCfg.GCPercent > 0 {
			cfg.GCPercent = fileCfg.GCPercent
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
		if explicitFlags["tls-cert"] {
			cfg.TLSCert = *tlsCert
		}
		if explicitFlags["tls-key"] {
			cfg.TLSKey = *tlsKey
		}
		if explicitFlags["alert-webhook"] {
			cfg.AlertWebhook = *alertWebhook
		}
		if explicitFlags["alert-error-rate"] {
			cfg.AlertErrorRate = *alertErrorRate
		}
		if explicitFlags["alert-latency"] {
			cfg.AlertLatency = *alertLatency
		}
		if explicitFlags["alert-window"] {
			cfg.AlertWindow = *alertWindow
		}
		if explicitFlags["gc-percent"] {
			cfg.GCPercent = *gcPercent
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

	// Set garbage collection aggressiveness target
	oldGC := debug.SetGCPercent(cfg.GCPercent)
	slog.Info("Garbage collector tuned", "gc_percent", cfg.GCPercent, "previous_gc_percent", oldGC)

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
	if cfg.SocketLocal != "" && cfg.SocketTarget != "" {
		modes++
	}

	if modes != 1 {
		fmt.Println("Usage: mcpwatch [mode] [--db path] [--ui port]")
		fmt.Println("\nYou must specify EXACTLY ONE mode:")
		fmt.Println("  --wrap \"command args\"   Run and intercept stdio of a local command")
		fmt.Println("  --proxy \"url\"           Proxy and intercept HTTP/SSE to a remote server")
		fmt.Println("  --pid 1234              Attach and intercept via eBPF to an existing process (Linux only)")
		fmt.Println("  --socket-local \"path\"   Local Unix socket path to listen on (socket mode)")
		fmt.Println("  --socket-target \"path\"  Target Unix socket path to forward to (socket mode)")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tp := tracing.InitTracer(ctx)
	if tp != nil {
		defer tp.Shutdown(context.Background())
	}

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	correlator := engine.NewCorrelator()

	// Start Alerting Engine
	if cfg.AlertWebhook != "" {
		alerter := alert.New(store, cfg.AlertWebhook, cfg.AlertErrorRate, cfg.AlertLatency, cfg.AlertWindow)
		go alerter.Start(ctx)
	}

	hub := server.NewHub()
	srv := server.New(store, hub, web.Assets)
	if cfg.AuthUser != "" && cfg.AuthPass != "" {
		srv.SetAuth(cfg.AuthUser, cfg.AuthPass)
	}

	go func() {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			if err := srv.StartTLS(cfg.UIPort, cfg.TLSCert, cfg.TLSKey); err != nil {
				slog.Error("server stopped", "error", err)
			}
		} else {
			if err := srv.Start(cfg.UIPort); err != nil {
				slog.Error("server stopped", "error", err)
			}
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
	} else if cfg.SocketLocal != "" && cfg.SocketTarget != "" {
		handlerTrans = transport.NewSocket(cfg.SocketLocal, cfg.SocketTarget, parser)
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
			metrics.RecordMetrics(msg)

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
