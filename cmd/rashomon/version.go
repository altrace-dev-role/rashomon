package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
)

// buildVersionOnce holds what buildVersion returns, computed on first use and
// never at init (RES-2).
var buildVersionOnce struct {
	sync.Once
	v string
}

// buildVersion is the version this binary reports, on `version`, in every
// record's writer and in the consent record. A build carrying ldflags never
// reads the build info.
func buildVersion() string {
	buildVersionOnce.Do(func() {
		if version != "" && version != "dev" {
			buildVersionOnce.v = version
			return
		}
		bi, ok := debug.ReadBuildInfo()
		buildVersionOnce.v = resolveVersion(version, bi, ok)
	})
	return buildVersionOnce.v
}

// resolveVersion decides the version: a value the linker set wins ("" and the
// default "dev" are not set); next a tagged module version from the build
// info, with its leading v dropped to match the GoReleaser form (v1.2.0-alpha.1
// is 1.2.0-alpha.1); otherwise dev. The build info of a local `go build` says
// "(devel)", which names no release.
func resolveVersion(ldflags string, bi *debug.BuildInfo, ok bool) string {
	if ldflags != "" && ldflags != "dev" {
		return ldflags
	}
	if ok && bi != nil {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

// printVersion is version's whole body, in the shape noArgs takes.
func printVersion(stdout io.Writer) error {
	fmt.Fprintln(stdout, buildVersion())
	return nil
}
