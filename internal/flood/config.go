package flood

import (
	"strconv"
	"time"
)

// Settings keys for flood control.
const (
	KeyStrategy       = "flood.strategy"
	KeyDedupeWindow   = "flood.dedupe_window"
	KeyRateN          = "flood.rate_n"
	KeyRateWindow     = "flood.rate_window"
	KeyDigestInterval = "flood.digest_interval"
)

// ParseConfig builds a Config from the settings map, falling back to
// DefaultConfig values for any missing or unparseable entry.
func ParseConfig(settings map[string]string) Config {
	d := DefaultConfig()
	if v := settings[KeyStrategy]; v != "" {
		d.Strategy = v
	}
	if v, err := time.ParseDuration(settings[KeyDedupeWindow]); err == nil {
		d.DedupeWindow = v
	}
	if v, err := strconv.Atoi(settings[KeyRateN]); err == nil && v > 0 {
		d.RateN = v
	}
	if v, err := time.ParseDuration(settings[KeyRateWindow]); err == nil {
		d.RateWindow = v
	}
	if v, err := time.ParseDuration(settings[KeyDigestInterval]); err == nil {
		d.DigestInterval = v
	}
	return d
}
