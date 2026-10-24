package version

// Version and BuildDate are overridable via -ldflags, e.g.:
//
//	-ldflags "-X tns65/internal/version.Version=1.0.0 -X tns65/internal/version.BuildDate=2026-10-24"
var (
	Version   string = "dev"
	BuildDate string = "unknown"
)
