package acceptance

// The installer places the binaries the CLI archive holds -- rashomon, and
// pgraph from a release that carries it -- and nothing else, whatever the
// release carries.
//
// install.sh is the first thing a stranger runs, and until this test nothing in
// the repository executed it: its behaviour was checked by `bash -n` and by
// reading. The alpha rule it must keep is that the closed observing proxy is
// never fetched -- an earlier version downloaded an `altrace_*` archive
// whenever a release happened to carry one -- so the release below carries
// one, and the test asserts it was never asked for.
//
// Nothing here reaches a network. A stand-in fetcher first on PATH serves every
// URL from a local directory by its base name and logs what was requested; the
// installer's real checksum, tar and install steps run unchanged, into a
// temporary directory. It runs twice, because install.sh has two fetchers:
// curl when it is on PATH, and wget when curl is not -- a branch that nothing
// would otherwise execute until a user without curl did.

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// curlShim answers the two shapes install.sh calls curl in -- `-o DEST URL`
// and `URL` to stdout -- plus a HEAD probe, from $SHIM_RELEASE, logging every
// URL to $SHIM_LOG. An unknown asset is a 404, as a real release host gives.
const curlShim = `#!/bin/sh
dest=""; url=""; head=0
while [ $# -gt 0 ]; do
  case "$1" in
    -o) dest="$2"; shift 2; continue ;;
    -I|--head) head=1 ;;
    -*) ;;
    *) url="$1" ;;
  esac
  shift
done
printf '%s\n' "$url" >> "$SHIM_LOG"
src="$SHIM_RELEASE/${url##*/}"
[ -f "$src" ] || exit 22
[ "$head" = 1 ] && exit 0
if [ -n "$dest" ]; then cp "$src" "$dest"; else cat "$src"; fi
`

// wgetShim answers the two shapes install.sh calls wget in -- `-qO DEST URL`
// and `-qO- URL` to stdout -- in the same way. wget's own exit status for a
// server error is 8.
const wgetShim = `#!/bin/sh
dest=""; url=""
while [ $# -gt 0 ]; do
  case "$1" in
    -qO-|-O-) dest="-" ;;
    -qO|-O) dest="$2"; shift 2; continue ;;
    -*) ;;
    *) url="$1" ;;
  esac
  shift
done
printf '%s\n' "$url" >> "$SHIM_LOG"
src="$SHIM_RELEASE/${url##*/}"
[ -f "$src" ] || exit 8
if [ -z "$dest" ] || [ "$dest" = "-" ]; then cat "$src"; else cp "$src" "$dest"; fi
`

// installerTools is every program install.sh and the wget stand-in run, for
// the PATH that has no curl on it. Missing ones are skipped: a platform
// carries shasum or sha256sum, not necessarily both, and tar calls gzip only
// where it has no gzip of its own.
var installerTools = []string{"uname", "mktemp", "rm", "grep", "cut", "head", "sed", "tar",
	"gzip", "shasum", "sha256sum", "perl", "mkdir", "cp", "chmod", "mv", "cat"}

// alphaClosing is the installer's last word on what the alpha does not do.
const alphaClosing = "Network traffic is not observed. The report lists the hosts each command"

// noPgraphNote and pgraphGoInstall are the two lines install.sh prints when
// the archive carries no pgraph, as every release up to v1.0-AutoSteer does.
// The second is README's own install line for pgraph, indented as there.
const (
	noPgraphNote    = "  this release carries no pgraph; a Go repository needs it (Go 1.26 or later):"
	pgraphGoInstall = "    go install github.com/altrace-dev-role/rashomon/pgraph/cmd/pgraph@main"
)

// cliWithPgraph is the CLI archive as a release with pgraph builds it.
var cliWithPgraph = map[string]int64{"rashomon": 0o755, "pgraph": 0o755}

// TestInstall_PlacesRashomonAndPgraph: from a release whose archive holds
// both binaries, the installer places rashomon and pgraph and nothing else,
// never fetches the proxy's archive, and does not say pgraph is missing.
// Break: leave the pgraph copy out of install.sh.
func TestInstall_PlacesRashomonAndPgraph(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("install.sh supports macOS and Linux only")
	}
	// install.sh names its archive by `uname -m`, which it maps to amd64 or
	// arm64 and refuses otherwise. On any other architecture it exits before
	// fetching, and a failure here would be the installer being right.
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("install.sh ships amd64 and arm64 only, not %s", runtime.GOARCH)
	}

	const version = "0.0.1"
	release, cli, proxy := installRelease(t, version, cliWithPgraph)
	check := func(t *testing.T, run installRun) {
		t.Helper()
		assertPlaced(t, run, cli, proxy, "pgraph", "rashomon")
		if strings.Contains(run.out, noPgraphNote) {
			t.Errorf("the installer says the release carries no pgraph although it placed one:\n%s", run.out)
		}
	}

	t.Run("curl", func(t *testing.T) {
		shims := t.TempDir()
		writeShim(t, shims, "curl", curlShim)
		// The stand-in first, then the real PATH: everything else the
		// installer runs is the machine's own.
		check(t, runInstaller(t, release, version,
			shims+string(os.PathListSeparator)+os.Getenv("PATH")))
	})

	t.Run("wget, with no curl on PATH", func(t *testing.T) {
		// A PATH built from nothing: the wget stand-in and a forwarder to each
		// tool the installer needs, so curl is absent rather than merely
		// shadowed. Forwarders, not symlinks: macOS's shasum is a perl wrapper
		// that reads its own path to choose a perl, and refuses to run under a
		// path it does not recognise.
		farm := t.TempDir()
		writeShim(t, farm, "wget", wgetShim)
		for _, tool := range installerTools {
			if real, err := exec.LookPath(tool); err == nil {
				writeShim(t, farm, tool, "#!/bin/sh\nexec "+shQuote(real)+" \"$@\"\n")
			}
		}
		if _, err := os.Stat(filepath.Join(farm, "curl")); err == nil {
			t.Fatal("premise broken: the wget-only PATH carries a curl")
		}
		check(t, runInstaller(t, release, version, farm))
	})
}

// TestInstall_AnOlderReleaseWithoutPgraph: v1.0-AutoSteer and every release
// before it carry rashomon alone, and RASHOMON_VERSION can still name them.
// The installer places rashomon, exits 0, and says in two lines of at most 80
// columns that this release has no pgraph and how to install it, in README's
// words ("A Go repository needs one step first", Go 1.26 or later, the
// go install line). Break: die on a missing pgraph, copy pgraph without
// looking for it, or drop the go install line.
func TestInstall_AnOlderReleaseWithoutPgraph(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("install.sh installs on macOS and Linux, amd64 and arm64")
	}
	const version = "0.0.3"
	release, cli, proxy := installRelease(t, version, map[string]int64{"rashomon": 0o755})
	shims := t.TempDir()
	writeShim(t, shims, "curl", curlShim)
	run := runInstaller(t, release, version, shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	assertPlaced(t, run, cli, proxy, "rashomon")
	lines := strings.Split(run.out, "\n")
	for _, want := range []string{noPgraphNote, pgraphGoInstall} {
		if !slices.Contains(lines, want) {
			t.Errorf("the installer does not print the line %q:\n%s", want, run.out)
		}
	}
	for _, l := range lines {
		if n := len([]rune(l)); strings.Contains(l, "pgraph") && n > 80 {
			t.Errorf("a line about pgraph is %d columns, want at most 80: %q", n, l)
		}
	}
}

// TestInstall_AFailedCopyLeavesNothingStaged: when the pgraph copy fails,
// after rashomon was staged under its temporary name, the installer exits
// non-zero and the install directory holds nothing -- neither binary and
// neither temporary name -- because install.sh's exit trap removes both
// temporary names. The archive's pgraph has mode 000, so cp cannot read it.
// Break: let the trap remove the download folder alone, as it did before
// pgraph joined the archive.
func TestInstall_AFailedCopyLeavesNothingStaged(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("install.sh installs on macOS and Linux, amd64 and arm64")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 file, so the copy this test needs to fail would not")
	}
	const version = "0.0.4"
	release, _, _ := installRelease(t, version, map[string]int64{"rashomon": 0o755, "pgraph": 0})
	shims := t.TempDir()
	writeShim(t, shims, "curl", curlShim)
	run, err := installer(t, release, version, shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err == nil {
		t.Fatalf("install.sh succeeded although pgraph could not be copied:\n%s", run.out)
	}
	if !strings.Contains(run.out, "could not write to") {
		t.Fatalf("premise broken: the installer failed before the copies:\n%s", run.out)
	}
	if len(run.placed) != 0 {
		t.Errorf("after a failed copy the install directory holds %v, want nothing:\n%s", run.placed, run.out)
	}
}

// installRelease writes a release of version for this machine into a new
// directory: the CLI's archive holding members (name to file mode), a proxy
// archive the installer must never ask for, and checksums.txt naming both.
func installRelease(t *testing.T, version string, members map[string]int64) (release, cli, proxy string) {
	t.Helper()
	release = t.TempDir()
	cli = fmt.Sprintf("rashomon_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	proxy = fmt.Sprintf("altrace_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	var sums strings.Builder
	for name, files := range map[string]map[string]int64{cli: members, proxy: {"altrace": 0o755}} {
		sum := writeArchive(t, filepath.Join(release, name), files)
		fmt.Fprintf(&sums, "%s  %s\n", sum, name)
	}
	if err := os.WriteFile(filepath.Join(release, "checksums.txt"), []byte(sums.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return release, cli, proxy
}

// TestInstall_TheLatestReleaseNeedsNoAPI (INSTALL-NOTOOLCHAIN1): with no
// RASHOMON_VERSION, the path a stranger takes, the installer reads the
// latest release's checksums.txt from the release host and takes the
// version from the archive it names, under both fetchers. It never asks
// api.github.com, which allows 60 unauthenticated requests an hour per
// address: a shared office address on launch day would leave strangers
// with no install. Break: resolve the tag through the API again.
func TestInstall_TheLatestReleaseNeedsNoAPI(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("install.sh installs on macOS and Linux, amd64 and arm64")
	}
	release, cli, proxy := installRelease(t, "0.0.2", cliWithPgraph)
	shims := t.TempDir()
	writeShim(t, shims, "curl", curlShim)
	run := runInstaller(t, release, "", shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	assertPlaced(t, run, cli, proxy, "pgraph", "rashomon")
	want := []string{"https://github.com/altrace-dev-role/rashomon/releases/latest/download/checksums.txt",
		"https://release.invalid/download/v0.0.2/" + cli}
	for _, w := range want {
		if !slices.Contains(run.requested, w) {
			t.Errorf("the installer never asked for %s:\n%s", w, strings.Join(run.requested, "\n"))
		}
	}
	for _, u := range run.requested {
		if strings.Contains(u, "api.github.com") {
			t.Errorf("the installer asked the API, %s", u)
		}
	}
}

// TestInstall_FailuresSayWhatToDoWithoutGo (INSTALL-NOTOOLCHAIN1): a latest
// release the installer cannot read names a tag to set, never a toolchain
// to install; and Windows (Git Bash, MSYS, Cygwin) is told it is not
// supported yet and pointed at WSL, never that the release ships no Windows
// archive, which it does. Every line fits 80 columns. Break: suggest
// `go install` again, or let any uname outside macOS and Linux read alike.
func TestInstall_FailuresSayWhatToDoWithoutGo(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("install.sh runs under a POSIX sh")
	}
	empty := t.TempDir()
	for _, c := range []struct {
		name, uname string
		want, not   []string
	}{
		{"no latest release readable", "", []string{"RASHOMON_VERSION=<tag>", "github.com/altrace-dev-role/rashomon/releases"},
			[]string{"go install"}},
		{"Git Bash", "MINGW64_NT-10.0-22631", []string{"Windows is not supported yet", "WSL"},
			[]string{"this release ships macOS and Linux"}},
		{"Cygwin", "CYGWIN_NT-10.0-22631", []string{"Windows is not supported yet", "WSL"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			shims := t.TempDir()
			writeShim(t, shims, "curl", curlShim)
			if c.uname != "" {
				writeShim(t, shims, "uname", "#!/bin/sh\ncase \"$1\" in -s) echo "+c.uname+" ;; *) echo x86_64 ;; esac\n")
			}
			run, err := installer(t, empty, "", shims+string(os.PathListSeparator)+os.Getenv("PATH"))
			if err == nil {
				t.Fatalf("install.sh succeeded with nothing to install:\n%s", run.out)
			}
			for _, w := range c.want {
				if !strings.Contains(run.out, w) {
					t.Errorf("the installer's output does not say %q:\n%s", w, run.out)
				}
			}
			for _, n := range c.not {
				if strings.Contains(run.out, n) {
					t.Errorf("the installer's output says %q:\n%s", n, run.out)
				}
			}
			for _, l := range strings.Split(run.out, "\n") {
				if n := len([]rune(l)); n > 80 {
					t.Errorf("a line is %d columns, want at most 80: %q", n, l)
				}
			}
		})
	}
}

// installRun is what one installer run left behind.
type installRun struct {
	out       string   // stdout and stderr together
	placed    []string // names in the install directory
	requested []string // every URL the stand-in fetcher was asked for
}

// runInstaller runs install.sh against the local release with the given PATH.
func runInstaller(t *testing.T, release, version, pathEnv string) installRun {
	t.Helper()
	run, err := installer(t, release, version, pathEnv)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, run.out)
	}
	return run
}

// installer is runInstaller that hands back the installer's failure. An
// empty version leaves RASHOMON_VERSION unset, the path a stranger takes.
func installer(t *testing.T, release, version, pathEnv string) (installRun, error) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "requests.log")
	installDir := filepath.Join(t.TempDir(), "bin")

	cmd := exec.Command("sh", filepath.Join(moduleRoot, "install.sh"))
	cmd.Env = append(os.Environ(),
		"PATH="+pathEnv,
		"HOME="+t.TempDir(),
		"RASHOMON_VERSION=",
		"RASHOMON_BASE_URL=https://release.invalid/download",
		"RASHOMON_INSTALL_DIR="+installDir,
		"SHIM_RELEASE="+release,
		"SHIM_LOG="+log,
	)
	if version != "" {
		cmd.Env = append(cmd.Env, "RASHOMON_VERSION=v"+version)
	}
	out, runErr := cmd.CombinedOutput()
	run := installRun{out: string(out)}
	// Read even after a failure: what a failed install leaves is the point
	// of TestInstall_AFailedCopyLeavesNothingStaged. A failure before mkdir
	// leaves no directory, which reads as nothing placed.
	entries, err := os.ReadDir(installDir)
	if err != nil && (runErr == nil || !os.IsNotExist(err)) {
		t.Fatal(err)
	}
	for _, e := range entries {
		run.placed = append(run.placed, e.Name())
	}
	if runErr != nil {
		return run, runErr
	}
	requested, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the stand-in fetcher logged nothing, so it was not the one used: %v\n%s", err, out)
	}
	run.requested = strings.Fields(string(requested))
	return run, nil
}

// assertPlaced holds one run to the alpha's installer contract: the install
// directory holds exactly want, in name order (a temporary name left behind
// is caught too), and only the CLI archive and checksums.txt were fetched.
func assertPlaced(t *testing.T, run installRun, cli, proxy string, want ...string) {
	t.Helper()
	if !slices.Equal(run.placed, want) {
		t.Errorf("install dir holds %v, want %v", run.placed, want)
	}

	// By NAME, never by the substring "altrace": the release repository's own
	// slug contains it, so a URL-wide substring match fails on a correct
	// installer the day a test stops overriding the base URL. And by an
	// allowlist as well as by the one name, so a proxy fetched under any
	// other name is caught too.
	var sawCLI bool
	for _, u := range run.requested {
		switch name := path.Base(u); name {
		case cli:
			sawCLI = true
		case "checksums.txt":
		case proxy:
			t.Errorf("the installer asked for the proxy's archive %s although the alpha never "+
				"fetches it:\n%s", proxy, strings.Join(run.requested, "\n"))
		default:
			t.Errorf("the installer asked for %s, which is neither the CLI archive nor the "+
				"checksums:\n%s", name, strings.Join(run.requested, "\n"))
		}
	}
	if !sawCLI {
		t.Errorf("the installer never asked for %s; the stand-in fetcher was not the one used:\n%s",
			cli, strings.Join(run.requested, "\n"))
	}

	if m := removedCommandMentions(run.out); len(m) > 0 {
		t.Errorf("the installer's closing lines point at commands that no longer exist %q:\n%s", m, run.out)
	}
	if !strings.Contains(run.out, alphaClosing) {
		t.Errorf("the installer does not say %q, so a new user is not told what the alpha "+
			"leaves out:\n%s", alphaClosing, run.out)
	}
}

// writeShim writes an executable stand-in named name into dir.
func writeShim(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

// writeArchive writes a .tar.gz holding one small script per member, each
// with its mode, in name order, and returns the archive's SHA-256, as a
// release's checksums.txt lists it.
func writeArchive(t *testing.T, dest string, members map[string]int64) string {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\nexit 0\n")
	for _, name := range slices.Sorted(maps.Keys(members)) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: members[name], Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []interface{ Close() error }{tw, gz, f} {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// shQuote wraps a path for the POSIX shell the child command is handed to. It
// came from the env test, which retired with the proxy era; the installer test
// is its one remaining user.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// removedCommandMentions lists the proxy-era commands a text still names. They
// went with the proxy era, and the one place a new user reads first must not
// send them to a command that answers "unknown command".
func removedCommandMentions(text string) []string {
	var out []string
	for _, m := range []string{"rashomon env", "rashomon run", "--proxy-store"} {
		if strings.Contains(text, m) {
			out = append(out, m)
		}
	}
	return out
}
