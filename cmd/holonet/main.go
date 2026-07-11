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

	"net/http"

	"github.com/t0mer/holonet/internal/api"
	"github.com/t0mer/holonet/internal/auth"
	"github.com/t0mer/holonet/internal/config"
	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/flood"
	"github.com/t0mer/holonet/internal/notify"
	"github.com/t0mer/holonet/internal/pipeline"
	"github.com/t0mer/holonet/internal/rules"
	"github.com/t0mer/holonet/internal/snmp"
	"github.com/t0mer/holonet/internal/store"
	"github.com/t0mer/holonet/internal/version"
	"github.com/t0mer/holonet/internal/webui"
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

	return runDaemon(bs, st, sealer, log)
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

// runDaemon starts the trap pipeline and the web/API server, blocking until a
// shutdown signal.
func runDaemon(bs config.Bootstrap, st *store.Store, sealer *crypto.Sealer, log *slog.Logger) error {
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
	engine := rules.New(st)
	dispatcher := notify.NewDispatcher(sealer, 10*time.Second, 2)

	// Flood controller — config from settings, reloadable at runtime.
	settings, _ := st.AllSettings()
	var proc *pipeline.Processor
	fc := flood.New(flood.ParseConfig(settings), func(r flood.Rollup) { proc.FloodFlush(ctx)(r) })
	proc = pipeline.New(st, decoder, engine, fc, dispatcher, log)
	go fc.Start(ctx.Done(), time.Second, time.Now)
	log.Info("flood control active", "strategy", fc.Strategy())

	traps := make(chan snmp.RawTrap, 256)
	go proc.Run(ctx, traps)

	// Web/API server.
	authMgr := auth.New(st, auth.SigningKeyFromMaster(bs.MasterKey), 24*time.Hour, false)
	apiSrv := api.New(api.Deps{
		Store:    st,
		Sealer:   sealer,
		Auth:     authMgr,
		Dispatch: dispatcher,
		Replay:   replayFunc(st, proc),
		ReloadFlood: func() {
			s, _ := st.AllSettings()
			fc.Configure(flood.ParseConfig(s))
			log.Info("flood control reconfigured", "strategy", fc.Strategy())
		},
		Version: version.Version,
		SPA:     webui.DistFS(),
	})
	httpSrv := &http.Server{Addr: bs.HTTPAddr, Handler: apiSrv.Handler()}
	go func() {
		log.Info("web/api server listening", "addr", bs.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			stop()
		}
	}()

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
	stop()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
	return nil
}

// replayFunc reconstructs a stored trap into a RawTrap and re-runs the pipeline,
// returning the newly stored trap id (design §3.7 replay).
func replayFunc(st *store.Store, proc *pipeline.Processor) api.Replayer {
	return func(ctx context.Context, trapID int64) (int64, error) {
		tv, err := st.GetTrap(trapID)
		if err != nil {
			return 0, err
		}
		var vbs []snmp.Varbind
		_ = json.Unmarshal([]byte(tv.VarbindsJSON), &vbs)
		raw := snmp.RawTrap{
			ReceivedAt: time.Now(),
			SourceIP:   tv.SourceIP,
			Version:    tv.SNMPVersion,
			TrapOID:    tv.TrapOID,
			Varbinds:   vbs,
		}
		return proc.Process(ctx, raw)
	}
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
