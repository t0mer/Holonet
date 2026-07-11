// Command holonet is the SNMP trap → messaging bridge daemon.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/term"

	"github.com/t0mer/holonet/internal/api"
	"github.com/t0mer/holonet/internal/auth"
	"github.com/t0mer/holonet/internal/config"
	"github.com/t0mer/holonet/internal/crypto"
	"github.com/t0mer/holonet/internal/decode"
	"github.com/t0mer/holonet/internal/flood"
	"github.com/t0mer/holonet/internal/metrics"
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

	// Service control (install/uninstall/start/stop/restart) needs neither the
	// database nor the master key.
	if bs.Service != "" {
		svc, err := newService(bs, noopProgram{})
		if err != nil {
			return err
		}
		if bs.MasterKey == "" {
			log.Warn("installing service without a master key; set HOLONET_MASTER_KEY in the service environment or reinstall with --master-key")
		}
		return service.Control(svc, bs.Service)
	}

	// Password reset needs the database but not the master key.
	if bs.ResetPassword {
		st, err := store.Open(bs.DBPath)
		if err != nil {
			return err
		}
		defer st.Close()
		return resetPassword(st, log)
	}

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

	// Run under kardianos/service so the same path serves a foreground run and a
	// managed OS service.
	svc, err := newService(bs, &program{bs: bs, st: st, sealer: sealer, log: log})
	if err != nil {
		return err
	}
	return svc.Run()
}

// newService builds the kardianos service with the daemon's flags baked in so an
// installed service starts with the same configuration.
func newService(bs config.Bootstrap, prg service.Interface) (service.Service, error) {
	return service.New(prg, &service.Config{
		Name:        "holonet",
		DisplayName: "HoloNet",
		Description: "SNMP trap → messaging bridge",
		Arguments:   serviceArgs(bs),
	})
}

// serviceArgs reconstructs the daemon flags for the installed unit. The master
// key is included when present (visible in the service definition); operators
// who prefer to keep it out can install without it and set HOLONET_MASTER_KEY in
// the service environment instead.
func serviceArgs(bs config.Bootstrap) []string {
	args := []string{
		"--db-path", bs.DBPath,
		"--http-addr", bs.HTTPAddr,
		"--log-level", bs.LogLevel,
	}
	if bs.SecureCookies {
		args = append(args, "--secure-cookies")
	}
	if bs.MasterKey != "" {
		args = append(args, "--master-key", bs.MasterKey)
	}
	return args
}

// program adapts the daemon to the kardianos service lifecycle.
type program struct {
	bs     config.Bootstrap
	st     *store.Store
	sealer *crypto.Sealer
	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		if err := runDaemon(ctx, p.bs, p.st, p.sealer, p.log); err != nil {
			p.log.Error("daemon exited with error", "err", err)
		}
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	select {
	case <-p.done:
	case <-time.After(12 * time.Second):
		p.log.Warn("daemon shutdown timed out")
	}
	return nil
}

// noopProgram satisfies service.Interface for control-only invocations.
type noopProgram struct{}

func (noopProgram) Start(service.Service) error { return nil }
func (noopProgram) Stop(service.Service) error  { return nil }

// resetPassword interactively updates the admin password.
func resetPassword(st *store.Store, log *slog.Logger) error {
	username, _, err := st.GetAdmin()
	if err != nil {
		return fmt.Errorf("no admin account configured; complete first-run setup in the web UI first")
	}
	fmt.Printf("Resetting password for admin user %q\n", username)
	pw, err := promptPassword("New password (min 8 chars): ")
	if err != nil {
		return err
	}
	if len(pw) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	confirm, err := promptPassword("Confirm password: ")
	if err != nil {
		return err
	}
	if pw != confirm {
		return fmt.Errorf("passwords do not match")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := st.UpdateAdminPassword(username, hash); err != nil {
		return err
	}
	log.Info("admin password updated", "username", username)
	return nil
}

// promptPassword reads a password from the terminal without echo.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
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

// runDaemon starts the trap pipeline and the web/API server, blocking until the
// context is cancelled (by the service lifecycle) or a fatal error.
func runDaemon(parent context.Context, bs config.Bootstrap, st *store.Store, sealer *crypto.Sealer, log *slog.Logger) error {
	ctx, stop := context.WithCancel(parent)
	defer stop()

	allow, err := communityChecker(st, sealer, log)
	if err != nil {
		return err
	}

	bindAddr, err := st.GetSetting("snmp.bind_addr")
	if err != nil {
		bindAddr = "0.0.0.0:1162"
	}

	met := metrics.New()

	decoder := decode.New(st)
	engine := rules.New(st)
	dispatcher := notify.NewDispatcher(sealer, 10*time.Second, 2)

	// Flood controller — config from settings, reloadable at runtime.
	settings, _ := st.AllSettings()
	var proc *pipeline.Processor
	fc := flood.New(flood.ParseConfig(settings), func(r flood.Rollup) { proc.FloodFlush(ctx)(r) })
	proc = pipeline.New(st, decoder, engine, fc, dispatcher, log)
	proc.SetMetrics(met)
	if chans, err := st.ListEnabledChannels(); err == nil {
		met.SetActiveChannels(len(chans))
	}
	go fc.Start(ctx.Done(), time.Second, time.Now)
	log.Info("flood control active", "strategy", fc.Strategy())

	traps := make(chan snmp.RawTrap, 256)
	go proc.Run(ctx, traps)

	// Web/API server.
	authMgr := auth.New(st, auth.SigningKeyFromMaster(bs.MasterKey), 24*time.Hour, bs.SecureCookies)
	apiSrv := api.New(api.Deps{
		Store:    st,
		Sealer:   sealer,
		Auth:     authMgr,
		Dispatch: dispatcher,
		Replay:   replayFunc(st, proc),
		RuleTest: ruleTestFunc(st, decoder, engine),
		ReloadFlood: func() {
			s, _ := st.AllSettings()
			fc.Configure(flood.ParseConfig(s))
			log.Info("flood control reconfigured", "strategy", fc.Strategy())
		},
		Metrics: met.Handler(),
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

	sink := snmp.NewSink(snmp.Config{
		BindAddr:       bindAddr,
		AllowCommunity: allow,
		V3Users:        v3Users(st, sealer, log),
		Log:            log,
		Metrics:        met,
		Debug:          log.Enabled(ctx, slog.LevelDebug),
	})

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

// ruleTestFunc dry-runs the decoder + rule engine against a sample event without
// persisting or dispatching (design §3.8, Rules inline test).
func ruleTestFunc(st *store.Store, decoder *decode.Decoder, engine *rules.Engine) api.RuleTester {
	return func(_ context.Context, in api.RuleTestInput) (api.RuleTestResult, error) {
		raw := snmp.RawTrap{
			ReceivedAt: time.Now(),
			SourceIP:   in.SourceIP,
			Version:    "test",
			TrapOID:    in.TrapOID,
			Varbinds:   []snmp.Varbind{{OID: snmp.SnmpTrapOID, Type: "OID", Value: in.TrapOID}},
		}
		ev, err := decoder.Decode(raw)
		if err != nil {
			return api.RuleTestResult{}, err
		}
		if in.Message != "" {
			ev.Message = in.Message // let varbind-regex rules match the sample text
		}
		d, err := engine.Classify(ev)
		if err != nil {
			return api.RuleTestResult{}, err
		}
		res := api.RuleTestResult{
			ResolvedName:       ev.ResolvedName,
			Unmapped:           ev.Unmapped,
			SeverityID:         d.SeverityID,
			Matched:            d.Matched,
			MatchedRuleID:      d.MatchedRuleID,
			BypassFloodControl: d.BypassFloodControl,
			ChannelIDs:         d.ChannelIDs,
		}
		if d.SeverityID != nil {
			if sev, err := st.GetSeverity(*d.SeverityID); err == nil {
				res.SeverityName = sev.Name
			}
		}
		if d.MatchedRuleID != nil {
			if rule, err := st.GetRule(*d.MatchedRuleID); err == nil {
				res.MatchedRuleName = rule.Name
			}
		}
		for _, chID := range d.ChannelIDs {
			if ch, err := st.GetChannel(chID); err == nil {
				res.ChannelNames = append(res.ChannelNames, ch.Name)
			}
		}
		return res, nil
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

// v3Users loads and unseals the enabled SNMPv3 users into USM credentials for
// the sink. Passwords live only in memory. Users that fail to unseal (wrong
// master key) are skipped with a warning.
func v3Users(st *store.Store, sealer *crypto.Sealer, log *slog.Logger) []snmp.USMUser {
	rows, err := st.ListEnabledV3Users()
	if err != nil {
		log.Warn("loading v3 users", "err", err)
		return nil
	}
	out := make([]snmp.USMUser, 0, len(rows))
	for _, u := range rows {
		authPass, err := sealer.OpenString(u.AuthPassSealed)
		if err != nil {
			log.Warn("skipping v3 user that failed to unseal (wrong master key?)", "id", u.ID)
			continue
		}
		privPass := ""
		if u.PrivPassSealed != "" {
			if privPass, err = sealer.OpenString(u.PrivPassSealed); err != nil {
				log.Warn("skipping v3 user with unreadable privacy password", "id", u.ID)
				continue
			}
		}
		out = append(out, snmp.USMUser{
			Username:      u.Username,
			SecurityLevel: u.SecurityLevel,
			AuthProtocol:  u.AuthProtocol,
			AuthPass:      authPass,
			PrivProtocol:  u.PrivProtocol,
			PrivPass:      privPass,
			EngineID:      u.EngineID,
		})
	}
	return out
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
