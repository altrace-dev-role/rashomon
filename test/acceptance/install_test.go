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
const alphaClosing = "Network destinations are not observed in this alpha"

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

// installRun is what one installer run left behind.
type installRun struct {
	out       string   // stdout and stderr together
	placed    []string // names in the install directory
	requested []string // every URL the stand-in fetcher was asked for
}

// runInstaller runs install.sh against the local release with the given PATH.
func runInstaller(t *testing.T, release, version, pathEnv string) installRun {
	t.Helper()
	log := filepath.Join(t.TempDir(), "requests.log")
	installDir := filepath.Join(t.TempDir(), "bin")

	cmd := exec.Command("sh", filepath.Join(moduleRoot, "install.sh"))
	cmd.Env = append(os.Environ(),
		"PATH="+pathEnv,
		"HOME="+t.TempDir(),
		"RASHOMON_VERSION=v"+version,
		"RASHOMON_BASE_URL=https://release.invalid/download",
		"RASHOMON_INSTALL_DIR="+installDir,
		"SHIM_RELEASE="+release,
		"SHIM_LOG="+log,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}

	run := installRun{out: string(out)}
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
	return run
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

	if m := dormantMentions(run.out); len(m) > 0 {
		t.Errorf("the installer's closing lines point at the dormant proxy path %q:\n%s", m, run.out)
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
