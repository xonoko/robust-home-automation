// Package version holds build information, set at link time with -ldflags.
package version

import "runtime"

// Set with: -ldflags "-X github.com/xonoko/robust-home-automation/internal/version.Version=v1.2.3".
var (
	Version = "dev"
	Commit  = "unknown"
)

// Info describes the running build.
type Info struct {
	Version   string `json:"version" doc:"Release version"`
	Commit    string `json:"commit" doc:"Git commit the binary was built from"`
	GoVersion string `json:"goVersion" doc:"Go toolchain version"`
}

// Get returns the build information of the running binary.
func Get() Info {
	return Info{Version: Version, Commit: Commit, GoVersion: runtime.Version()}
}
