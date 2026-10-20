package version

// Version is overridden at build time via -ldflags.
var Version = "0.0.0-dev"

func String() string { return "fluxor " + Version }
