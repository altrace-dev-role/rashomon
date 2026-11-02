package acceptance

// The installer places the CLI and nothing else, whatever the release carries.
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

func TestInstall_PlacesOnlyTheCLI(t *testing.T) {
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
	release := t.TempDir()
	cli := fmt.Sprintf("rashomon_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	proxy := fmt.Sprintf("altrace_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	var sums strings.Builder
	for name, member := range map[string]string{cli: "rashomon", proxy: "altrace"} {
		sum := writeArchive(t, filepath.Join(release, name), member)
		fmt.Fprintf(&sums, "%s  %s\n", sum, name)
	}
	if err := os.WriteFile(filepath.Join(release, "checksums.txt"), []byte(sums.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("curl", func(t *testing.T) {
		shims := t.TempDir()
		writeShim(t, shims, "curl", curlShim)
		// The stand-in first, then the real PATH: everything else the
		// installer runs is the machine's own.
		assertOnlyTheCLI(t, runInstaller(t, release, version,
			shims+string(os.PathListSeparator)+os.Getenv("PATH")), cli, proxy)
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
		assertOnlyTheCLI(t, runInstaller(t, release, version, farm), cli, proxy)
	})
}

// installRelease writes a release of version for this machine into a new
// directory, as TestInstall_PlacesOnlyTheCLI's: the CLI's archive, a proxy
// archive the installer must never ask for, and checksums.txt naming both.
func installRelease(t *testing.T, version string) (release, cli, proxy string) {
	t.Helper()
	release = t.TempDir()
	cli = fmt.Sprintf("rashomon_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	proxy = fmt.Sprintf("altrace_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	var sums strings.Builder
	for name, member := range map[string]string{cli: "rashomon", proxy: "altrace"} {
		sum := writeArchive(t, filepath.Join(release, name), member)
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
	release, cli, proxy := installRelease(t, "0.0.2")
	shims := t.TempDir()
	writeShim(t, shims, "curl", curlShim)
	run := runInstaller(t, release, "", shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	assertOnlyTheCLI(t, run, cli, proxy)
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
	out, err := cmd.CombinedOutput()
	run := installRun{out: string(out)}
	if err != nil {
		return run, err
	}
	entries, err := os.ReadDir(installDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		run.placed = append(run.placed, e.Name())
	}
	requested, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the stand-in fetcher logged nothing, so it was not the one used: %v\n%s", err, out)
	}
	run.requested = strings.Fields(string(requested))
	return run, nil
}

// assertOnlyTheCLI holds one run to the alpha's installer contract.
func assertOnlyTheCLI(t *testing.T, run installRun, cli, proxy string) {
	t.Helper()
	if len(run.placed) != 1 || run.placed[0] != "rashomon" {
		t.Errorf("install dir holds %v, want only rashomon", run.placed)
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

// writeArchive writes a .tar.gz holding one executable file named member and
// returns the archive's SHA-256, as a release's checksums.txt lists it.
func writeArchive(t *testing.T, dest, member string) string {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := tw.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
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
