// Package version holds the build-injected version string.
package version

// Version is set at build time via -ldflags
// "-X github.com/t0mer/holonet/internal/version.Version=<v>".
var Version = "dev"
