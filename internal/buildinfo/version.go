package buildinfo

// Version is set from the binary's build information at startup.
var Version = "dev"

func UserAgent() string { return "rootly-cli/" + Version }
