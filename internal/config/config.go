// Package config resolves HoloNet's bootstrap configuration.
//
// Per the design (§6), SQLite is the source of truth for all operational config
// (sinks, devices, severities, OID map, channels, rules, routes, flood strategy,
// settings). Only the handful of values needed *before* the database opens come
// from flags/env via Viper, with precedence flags > env > built-in default.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Bootstrap holds the values needed before the store opens.
type Bootstrap struct {
	DBPath        string // sqlite database file path
	MasterKey     string // AES-GCM master key for secret sealing
	HTTPAddr      string // listen address for the web/API server
	LogLevel      string // debug|info|warning|error
	SecureCookies bool   // set the Secure flag on session cookies (enable behind TLS)

	// One-shot admin actions (Slice 1 bootstrap, before the UI exists). When
	// set, the binary performs the action and exits instead of running the
	// daemon. Superseded by the API/UI in Slice 2 but kept for scripting.
	AddCommunity string // seal + insert a v2c community string
	AddShoutrrr  string // "name=shoutrrr-url" — seal + insert a shoutrrr channel
}

// Defaults for bootstrap values.
const (
	DefaultDBPath   = "/data/holonet.db"
	DefaultHTTPAddr = ":8080"
	DefaultLogLevel = "info"
)

// Load parses flags and environment (HOLONET_ prefix) into a Bootstrap.
// It also wires --version handling for the caller via the returned showVersion
// flag value. args should be os.Args[1:].
func Load(args []string) (Bootstrap, bool, error) {
	fs := pflag.NewFlagSet("holonet", pflag.ContinueOnError)
	dbPath := fs.String("db-path", DefaultDBPath, "SQLite database file path")
	masterKey := fs.String("master-key", "", "master key for sealing secrets at rest")
	httpAddr := fs.String("http-addr", DefaultHTTPAddr, "listen address for the web/API server")
	logLevel := fs.String("log-level", DefaultLogLevel, "log level: debug|info|warning|error")
	secureCookies := fs.Bool("secure-cookies", false, "set the Secure flag on session cookies (enable when served over TLS)")
	showVersion := fs.Bool("version", false, "print version and exit")
	addCommunity := fs.String("add-community", "", "seal and insert a v2c community string, then exit")
	addShoutrrr := fs.String("add-shoutrrr", "", "insert a shoutrrr channel as \"name=url\", then exit")

	if err := fs.Parse(args); err != nil {
		return Bootstrap{}, false, err
	}
	if *showVersion {
		return Bootstrap{}, true, nil
	}

	v := viper.New()
	v.SetEnvPrefix("HOLONET")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()
	// Bind flags so an explicitly-set flag wins over env, but env overrides the
	// built-in default when the flag was left at its default.
	_ = v.BindPFlag("db_path", fs.Lookup("db-path"))
	_ = v.BindPFlag("master_key", fs.Lookup("master-key"))
	_ = v.BindPFlag("http_addr", fs.Lookup("http-addr"))
	_ = v.BindPFlag("log_level", fs.Lookup("log-level"))
	_ = v.BindPFlag("secure_cookies", fs.Lookup("secure-cookies"))

	bs := Bootstrap{
		DBPath:        firstNonEmpty(*dbPath, v.GetString("db_path"), DefaultDBPath),
		MasterKey:     firstNonEmpty(*masterKey, v.GetString("master_key")),
		HTTPAddr:      firstNonEmpty(*httpAddr, v.GetString("http_addr"), DefaultHTTPAddr),
		LogLevel:      firstNonEmpty(*logLevel, v.GetString("log_level"), DefaultLogLevel),
		SecureCookies: *secureCookies || v.GetBool("secure_cookies"),

		AddCommunity: *addCommunity,
		AddShoutrrr:  *addShoutrrr,
	}
	return bs, false, nil
}

// Validate checks that required bootstrap values are present.
func (b Bootstrap) Validate() error {
	if b.MasterKey == "" {
		return fmt.Errorf("master key is required: set --master-key or HOLONET_MASTER_KEY")
	}
	if b.DBPath == "" {
		return fmt.Errorf("db path is required")
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if s != "" {
			return s
		}
	}
	return ""
}
