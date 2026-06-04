package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/ai-sre/agent/internal/identity"
)

type Config struct {
	Dir    string
	Port   int
	Secret string
}

func envConfig() *Config {
	cfg := &Config{
		Dir:  "/var/lib/ai-server-agent",
		Port: 9090,
	}
	if v := os.Getenv("AGENT_DATA_DIR"); v != "" {
		cfg.Dir = v
	}
	if v := os.Getenv("AGENT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	cfg.Secret = os.Getenv("AGENT_SECRET")
	return cfg
}

func main() {
	cfg := envConfig()

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	serveCmd.StringVar(&cfg.Dir, "dir", cfg.Dir, "data directory")
	serveCmd.IntVar(&cfg.Port, "port", cfg.Port, "HTTP listen port")
	serveCmd.StringVar(&cfg.Secret, "secret", cfg.Secret, "shared secret for API auth")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s serve [flags]\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		serveCmd.Parse(os.Args[2:])
		if cfg.Secret == "" {
			fmt.Fprintln(os.Stderr, "error: AGENT_SECRET env or --secret is required")
			os.Exit(1)
		}
		if err := run(cfg); err != nil {
			log.Fatalf("fatal: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func run(cfg *Config) error {
	// 初始化 identity
	if _, err := identity.New(cfg.Dir); err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	srv := newServer(cfg, ln)

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down", sig)
		srv.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", ln.Addr().String())
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return fmt.Errorf("http: %w", err)
	}
	return nil
}

func newServer(cfg *Config, ln net.Listener) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/v1/inspect", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"not yet implemented"}`))
	})

	mux.Handle("/api/", authMiddleware(cfg.Secret, apiMux))

	return &http.Server{Handler: mux}
}

func authMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
