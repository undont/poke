// package version carries build identity for poke/poked
package version

// overridden at build time with -ldflags "-X .../version.Version=..."
var (
	Version  = "0.1.0-dev"
	Protocol = 1
)
