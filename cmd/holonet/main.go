// Command holonet is the SNMP trap → messaging bridge daemon.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/t0mer/holonet/internal/config"
	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/pipeline"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
	"github.com/t0mer/holonet/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "holonet: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	bs, showVersion, err := config.Load(args)
	if err != nil {
		return err
	}
	if showVersion {
		fmt.Println(version.Version)
		return nil
	}

	log := newLogger(bs.LogLevel)
	slog.SetDefault(log)

	if err := bs.Validate(); err != nil {
		return err
	}

	st, err := store.Open(bs.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	sealer, err := crypto.New(bs.MasterKey)
	if err != nil {
		return err
	}

	// One-shot bootstrap admin actions (Slice 1, pre-UI).
	if bs.AddCommunity != "" || bs.AddShoutrrr != "" {
		return runAdmin(bs, st, sealer, log)
	}

	return runDaemon(st, sealer, log)
}

// runAdmin performs the requested one-shot bootstrap action(s) and returns.
func runAdmin(bs config.Bootstrap, st *store.Store, sealer *crypto.Sealer, log *slog.Logger) error {
	if bs.AddCommunity != "" {
		sealed, err := sealer.SealString(bs.AddCommunity)
		if err != nil {
			return fmt.Errorf("seal community: %w", err)
		}
		id, err := st.AddCommunity(sealed, true)
		if err != nil {
			return err
		}
		log.Info("added v2c community", "id", id)
	}
	if bs.AddShoutrrr != "" {
		name, url, ok := strings.Cut(bs.AddShoutrrr, "=")
		if !ok || name == "" || url == "" {
			return fmt.Errorf("--add-shoutrrr must be \"name=url\"")
		}
		cfg, _ := json.Marshal(notify.ShoutrrrConfig{URL: url})
		sealed, err := sealer.SealString(string(cfg))
		if err != nil {
			return fmt.Errorf("seal channel config: %w", err)
		}
		id, err := st.AddChannel(name, notify.KindShoutrrr, sealed, true)
		if err != nil {
			return err
		}
		log.Info("added shoutrrr channel", "id", id, "name", name)
	}
	return nil
}

// runDaemon starts the trap pipeline and blocks until a shutdown signal.
func runDaemon(st *store.Store, sealer *crypto.Sealer, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	allow, err := communityChecker(st, sealer, log)
	if err != nil {
		return err
	}

	bindAddr, err := st.GetSetting("snmp.bind_addr")
	if err != nil {
		bindAddr = "0.0.0.0:1162"
	}

	decoder := decode.New(st)
	dispatcher := notify.NewDispatcher(sealer, 10*time.Second, 2)
	proc := pipeline.New(st, decoder, dispatcher, log)

	traps := make(chan snmp.RawTrap, 256)
	go proc.Run(ctx, traps)

	sink := snmp.NewV2CSink(bindAddr, allow, log, snmp.NopMetrics{})

	log.Info("holonet starting", "version", version.Version, "bind", bindAddr)
	errCh := make(chan error, 1)
	go func() { errCh <- sink.Start(ctx, traps) }()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return err
		}
	}
	// Give the sink a moment to unwind on graceful shutdown.
	stop()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
	return nil
}

// communityChecker loads and unseals the enabled v2c communities into a set and
// returns a membership predicate. The plaintext strings live only in memory.
func communityChecker(st *store.Store, sealer *crypto.Sealer, log *slog.Logger) (func(string) bool, error) {
	rows, err := st.ListEnabledCommunities()
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(rows))
	for _, c := range rows {
		plain, err := sealer.OpenString(c.CommunitySealed)
		if err != nil {
			log.Warn("skipping community that failed to unseal (wrong master key?)", "id", c.ID)
			continue
		}
		set[plain] = struct{}{}
	}
	if len(set) == 0 {
		log.Warn("no enabled v2c communities configured; all v2c traps will be dropped")
	}
	return func(community string) bool {
		_, ok := set[community]
		return ok
	}, nil
}

// newLogger builds a slog logger at the requested level.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warning", "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
