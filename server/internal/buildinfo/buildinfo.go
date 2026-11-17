// Package buildinfo identifies the running build. Release and CI builds set
// both values with -ldflags -X; other builds report dev.
package buildinfo

var (
	Version = "dev"
	Commit  = ""
)
