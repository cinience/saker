// Package synapse provides a built-in synapse hub client that allows saker
// to register directly with a synapse control plane via gRPC, eliminating
// the need for a separate saker-bridge binary.
package synapse

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"strings"
	"time"
)

// BridgeConfig holds the configuration for the built-in synapse bridge.
type BridgeConfig struct {
	HubAddr        string
	AuthToken      string
	InstanceID     string
	SandboxID      string
	Models         []string
	MaxConcurrent  int32
	Labels         map[string]string
	InsecureTLS    bool
	Heartbeat      time.Duration
	SakerBaseURL   string // base URL of saker's own HTTP server (default: http://127.0.0.1:<port>)
	Logger         *slog.Logger
}

// RunBridge starts the synapse hub registration loop. It blocks until ctx
// is cancelled. Designed to be called as a goroutine from saker's server
// startup:
//
//	go synapse.RunBridge(ctx, cfg)
func RunBridge(ctx context.Context, cfg BridgeConfig) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	if cfg.Heartbeat <= 0 {
		cfg.Heartbeat = 15 * time.Second
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 8
	}
	if cfg.SakerBaseURL == "" {
		cfg.SakerBaseURL = "http://127.0.0.1:17000"
	}

	backend := NewHTTPBackend(cfg.SakerBaseURL)

	var tlsCfg *tls.Config
	if !cfg.InsecureTLS {
		host := cfg.HubAddr
		if i := strings.LastIndex(host, ":"); i > 0 {
			host = host[:i]
		}
		tlsCfg = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}

	dialer := NewDialer(
		DialOptions{Addr: cfg.HubAddr, TLSConfig: tlsCfg},
		HelloOptions{
			InstanceID:    cfg.InstanceID,
			Models:        cfg.Models,
			MaxConcurrent: cfg.MaxConcurrent,
			AuthToken:     cfg.AuthToken,
			SandboxID:     cfg.SandboxID,
			Labels:        cfg.Labels,
			Version:       cfg.Version(),
		},
		logger.With("component", "synapse.dialer"),
	)

	for {
		if ctx.Err() != nil {
			return
		}
		session, err := dialer.ConnectWithBackoff(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("synapse connect aborted", "error", err)
			return
		}
		pump := NewPump(PumpOptions{
			Stream:    session.Stream(),
			Backend:   backend,
			Logger:    logger.With("component", "synapse.pump"),
			Heartbeat: cfg.Heartbeat,
		})
		if err := pump.Run(ctx); err != nil {
			logger.Warn("synapse pump exited; will reconnect", "error", err)
		} else {
			logger.Info("synapse pump exited cleanly; will reconnect")
		}
		_ = session.Close()

		reconnect := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			reconnect.Stop()
			return
		case <-reconnect.C:
		}
	}
}

// Version returns the saker version string. This is set at build time or
// falls back to "dev".
func (cfg BridgeConfig) Version() string {
	return "embedded"
}
