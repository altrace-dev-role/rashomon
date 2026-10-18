// Package baseline remembers which hosts a project's sessions have named
// before, so the report can say which are new.
//
// It lives OUTSIDE the run store on purpose. The run store is evictable -- a
// size cap or a `forget` removes runs -- and a novelty baseline that could be
// evicted would make every host novel again the first time the store filled
// up, turning retention into a flood of findings. The baseline is small
// (hostnames only) and is the one thing here that is meant to outlive the runs.
//
// Ownership is EARLIEST-SESSION-WINS rather than first-render-wins, and that is
// what makes a re-render stable. If novelty were decided by which render
// happened first, re-rendering an old session after a new one had run would
// move hosts between them and two renders of one session would differ. Keyed on
// the session's own start time instead, a session's novel set is a property of
// the session, and rendering out of chronological order corrects toward the
// earlier owner rather than toward the most recent reader.
package baseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version is carried by the file so a later reader can refuse a shape it does
// not know rather than guessing at it.
const Version = 1

// Basis is what a baseline's hosts are: the hosts a project's sessions NAMED
// in tool calls. Files from the proxy era held the hosts a session REACHED,
// as the proxy saw them, and were written only on sessions where the proxy
// store was readable. Comparing one of those against named hosts would call a
// host "named for the first time" when it was only never reached, so a file
// without this marker is read as absent and the project is established again,
// once, from the record.
const Basis = "declared"

// ubiquitous are hosts that are never novel, whatever the baseline says.
//
// Their first appearance in a project is not information: every Go, Node or
// Python project reaches these, and a novelty line that fired on them on its
// first run would fire on almost every first run and be ignored from then on.
// The list is short, fixed, and deliberately made of package registries and
// their CDNs rather than anything an operator might run themselves.
var ubiquitous = map[string]bool{
	"registry.npmjs.org":            true,
	"pypi.org":                      true,
	"files.pythonhosted.org":        true,
	"github.com":                    true,
	"api.github.com":                true,
	"objects.githubusercontent.com": true,
	"proxy.golang.org":              true,
	"sum.golang.org":                true,
	"formulae.brew.sh":              true,
	"ghcr.io":                       true,
}

// Ubiquitous reports whether a host is on the never-novel list. Exported so the
// report can say so rather than silently dropping the host.
func Ubiquitous(host string) bool { return ubiquitous[host] }

// entry is when a host was first named in this project, and by which session.
type entry struct {
	FirstSeenSession  string `json:"first_seen_session"`
	FirstSeenAtUnixMS int64  `json:"first_seen_at_unix_ms"`
}

// File is one project's baseline.
type File struct {
	Version int    `json:"version"`
	Project string `json:"project"`
	// Basis is what the hosts are; see the constant. Written on every save,
	// read as the file's identity: a file that does not say "declared" is not
	// this baseline.
	Basis string `json:"basis,omitempty"`
	// EstablishedBySession is the session that created this file.
	//
	// Recorded rather than inferred, because "did this session establish the
	// baseline" must be a property of the SESSION and not of the render. Deciding
	// it from whether the file existed made the first render of a session differ
	// from the second: the first created the file, so the second reported the
	// baseline as pre-existing and every host the session had reached read as
	// novel. A query that changes its own answer by being run is the worst shape
	// available for an instrument.
	//
	// Empty on a file written before this field existed, which reads as "nobody
	// established it" -- so novelty is computed normally rather than suppressed
	// for a session that cannot be identified as the first. The field is additive
	// and the version is unchanged, so such a file still loads.
	EstablishedBySession string `json:"established_by_session,omitempty"`
	// EstablishedAtUnixMS is that session's own start instant, which is what
	// lets the establisher correct toward an earlier session exactly as host
	// ownership does.
	//
	// Without it the establisher was first-RENDER-wins: a user who worked for a
	// week and then rendered one recent session gave that session the slot
	// permanently, and the project's chronologically first session afterwards
	// reported every host it reached as new -- while the recent one claimed to
	// have been first.
	EstablishedAtUnixMS int64            `json:"established_at_unix_ms,omitempty"`
	Hosts               map[string]entry `json:"hosts"`
}

// Result is what one session's render learned.
type Result struct {
	// Established is true when THIS SESSION created the baseline. The first
	// session in a project has no previous runs to be novel against, so every
	// host it named would otherwise be reported as new -- which is true and
	// useless. The report says "baseline established (N hosts)" instead.
	//
	// It stays true on every later render of that session, which is what makes a
	// re-render stable; see File.EstablishedBySession.
	Established bool
	// Novel are this session's hosts that no earlier session named, minus the
	// ubiquitous list. Sorted.
	Novel []string
	// Known is the size of the baseline AFTER this session -- a property of the
	// comparison at render time rather than of the session, and reported as such
	// ("N known"). It grows as other sessions run; what does not change is which
	// hosts this session owns.
	Known int
	// Path is the file consulted, for the report to name when it could not be
	// read.
	Path string
	// Err is set when the baseline could not be read or written. Novelty then
	// renders as unknown rather than as an empty list: "no new hosts" and "we
	// could not tell" are different answers.
	Err error
}

// Key identifies the project a session belongs to.
//
// The git work tree root when there is one, else the working directory. Symlinks
// are resolved first so that /tmp and /private/tmp on macOS -- the same
// directory under two names -- do not become two projects with two baselines
// and a flood of false novelty in each.
//
// The root is found by walking up for a .git entry rather than by running `git
// rev-parse`. That is not a shortcut: os/exec is forbidden in this program's
// dependency graph (H-17), and a recorder that can spawn a process can reach
// the network through one. Walking the tree needs no subprocess and works in a
// worktree, where .git is a file rather than a directory.
func Key(cwd string) string {
	if cwd == "" {
		return ""
	}
	dir := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		dir = resolved
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root with no .git: the directory itself
			// is the project.
			break
		}
		dir = parent
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}
	return cwd
}

// fileName turns a project key into one path element.
//
// The key is an absolute path, so it cannot be a filename as it stands. Every
// separator and every character that is not plainly safe becomes an underscore,
// and a digest of the full key is appended so that two different projects whose
// sanitised names collide still get two files. Without the digest, /a/b and
// /a-b would share a baseline.
func fileName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if len(name) > 80 {
		name = name[len(name)-80:]
	}
	return name + "-" + digest(key) + ".json"
}

// Update folds this session's hosts into the project's baseline and reports
// which were new.
//
// root is the store root; the baseline lives in a sibling directory of runs/ so
// that eviction of runs cannot touch it. sessionStart is the session's own
// start instant, which is what decides ownership.
func Update(root, key, sessionID string, sessionStart time.Time, hosts []string) Result {
	res := Result{}
	if root == "" || key == "" {
		res.Err = errors.New("baseline: no store root or project key")
		return res
	}
	dir := filepath.Join(root, "baseline")
	res.Path = filepath.Join(dir, fileName(key))

	f, existed, err := load(res.Path, key)
	if err != nil {
		res.Err = err
		return res
	}
	stamp := sessionStart.UTC().UnixMilli()
	switch {
	case !existed:
		f.EstablishedBySession = sessionID
		f.EstablishedAtUnixMS = stamp
	case f.EstablishedBySession != "" && stamp > 0 &&
		(f.EstablishedAtUnixMS == 0 || stamp < f.EstablishedAtUnixMS):
		// An earlier session than the recorded establisher. It was the project's
		// first, so it takes the slot -- the same earliest-wins correction the
		// per-host entries make, for the same reason: the answer must be a
		// property of the sessions and not of the order someone read them in.
		// Monotone, so it converges rather than oscillating.
		//
		// Guarded on a recorded establisher EXISTING. A file written before this
		// field names nobody, and "nobody established it" must not become "the
		// next session to render claims it" -- that would hand the slot, and the
		// suppression of every novelty finding that comes with it, to whoever
		// happened to run report first on an upgraded install.
		f.EstablishedBySession = sessionID
		f.EstablishedAtUnixMS = stamp
	}
	res.Established = f.EstablishedBySession != "" && f.EstablishedBySession == sessionID

	for _, h := range hosts {
		if h == "" {
			continue
		}
		prev, seen := f.Hosts[h]
		switch {
		case !seen:
			f.Hosts[h] = entry{FirstSeenSession: sessionID, FirstSeenAtUnixMS: stamp}
		case stamp > 0 && (prev.FirstSeenAtUnixMS == 0 || stamp < prev.FirstSeenAtUnixMS):
			// An earlier session is being rendered after a later one. The
			// earlier one owns the host; correcting toward it is what keeps a
			// session's novel set a property of the session rather than of the
			// order somebody read the reports in.
			f.Hosts[h] = entry{FirstSeenSession: sessionID, FirstSeenAtUnixMS: stamp}
		}
	}
	res.Known = len(f.Hosts)

	// Novel is computed from the file AFTER folding, so it is exactly "the
	// hosts this session owns" -- stable across renders by construction.
	for _, h := range hosts {
		if ubiquitous[h] {
			continue
		}
		if e, ok := f.Hosts[h]; ok && e.FirstSeenSession == sessionID && !res.Established {
			res.Novel = append(res.Novel, h)
		}
	}
	sort.Strings(res.Novel)
	res.Novel = dedupe(res.Novel)

	if err := save(dir, res.Path, f); err != nil {
		res.Err = err
	}
	return res
}

// load reads a project's baseline. existed is false for a file that is not
// there and for one without the basis marker, and the File is then a fresh
// one for the key.
func load(path, key string) (File, bool, error) {
	fresh := File{Version: Version, Project: key, Basis: Basis, Hosts: map[string]entry{}}
	f, err := parse(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fresh, false, nil
	}
	if err != nil {
		// A corrupt baseline is not a reason to fail a report, and it is not a
		// reason to silently treat every host as novel either. Starting a fresh
		// file loses history; saying so is the caller's job, so the error is
		// returned and novelty renders as unknown.
		return File{}, false, err
	}
	if f.Basis != Basis {
		// A proxy-era file: hosts reached, not hosts named. Not an error and
		// not a comparison; the project starts over from the record, and the
		// session doing it reports the baseline as established.
		return fresh, false, nil
	}
	if f.Hosts == nil {
		f.Hosts = map[string]entry{}
	}
	return f, true, nil
}

// parse reads one baseline file as written, whatever its basis says.
func parse(path string) (File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return decode(body)
}

// decode reads a baseline's bytes, refusing a version this build does not
// know rather than guessing at its shape.
func decode(body []byte) (File, error) {
	var f File
	if err := json.Unmarshal(body, &f); err != nil {
		return File{}, err
	}
	if f.Version != Version {
		return File{}, errors.New("baseline: unrecognised file version")
	}
	return f, nil
}

// save writes the baseline atomically, 0600 in a 0700 directory.
//
// A hostname is the most sensitive field this product holds, and a baseline is
// a list of every host a project has ever reached -- a longer-lived and more
// revealing artefact than any single run. It gets the same permissions as the
// store and the same rename-into-place write, so a crash mid-write cannot leave
// a truncated file that reads as "this project has reached almost nothing".
func save(dir, path string, f File) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	tmp, err := os.CreateTemp(dir, ".baseline-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// One deferred cleanup, as internal/settings and internal/store do it: it
	// covers every failure path below and is a no-op after a successful rename,
	// which is strictly safer than remembering to remove on each branch.
	defer os.Remove(name) //nolint:errcheck // best effort cleanup

	if _, err := tmp.Write(body); err != nil {
		tmp.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close() //nolint:errcheck // the sync error is what matters
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// digest is a short SHA-256 of the project key, appended to the sanitised
// filename so two projects whose names sanitise identically still get two
// files. It is not a secret and not keyed: the key it hashes is a path already
// on this machine, and its only job is collision resistance.
func digest(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

func dedupe(xs []string) []string {
	if len(xs) < 2 {
		return xs
	}
	out := xs[:1]
	for _, x := range xs[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

// ForgetFile removes a host from one baseline file by path, returning 1 when
// the file held it and 0 when it did not. A file this build will not rewrite
// -- one without the basis marker, or one it cannot parse whose bytes hold
// the host -- is deleted whole.
//
// By PATH rather than by project key, because `forget --host` is not told which
// project the caller meant and must clear the host from every baseline that
// remembers it. That is the conservative direction: a host the user asked to
// forget must not survive in a project they did not think to name.
func ForgetFile(path, host string) (int, error) {
	if path == "" || host == "" {
		return 0, nil
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Listed, then gone before it was read: nothing left to clear.
		return 0, nil
	}
	if err != nil {
		// An unreadable baseline may hold the host, so the forget cannot say
		// it cleared it. The caller reports the error; it is never a zero.
		return 0, err
	}
	f, err := decode(body)
	if err != nil {
		// Bytes that do not parse, or a version this build does not know,
		// cannot be rewritten without the host -- there is no shape here to
		// delete one entry from and write back. Holding it, the file goes
		// whole and the project establishes its baseline again, which is the
		// conservative direction: refusing would leave the forgotten host
		// sitting in a file this forget cannot touch any other way. Not
		// holding it, the file is no business of this forget.
		if !namesHost(body, host) {
			return 0, nil
		}
		return removeWhole(path)
	}
	_, held := f.Hosts[host]
	if f.Basis == Basis {
		if !held {
			return 0, nil
		}
		delete(f.Hosts, host)
		if err := save(filepath.Dir(path), path, f); err != nil {
			return 0, err
		}
		return 1, nil
	}
	// A file without the basis marker is not this baseline. load reads it as
	// absent, so removing it changes no report, and saving it would stamp the
	// marker onto hosts it held under the old basis -- but only when it holds
	// the host this forget was asked for. One that does not is no business of
	// this forget, and must not be deleted just for sharing the directory.
	if !held {
		return 0, nil
	}
	return removeWhole(path)
}

// namesHost reports whether a baseline's bytes hold the host as JSON writes
// it, between quotes, so a short host does not match inside another name or
// a key: --host io is not in "version". A recorded host is letters, digits,
// dots, hyphens, underscores, and -- for IPv6 -- brackets and colons, none of
// which JSON ever escapes, so the quoted form is exact. A host spelled like
// one of the file's own keys, such as hosts, still matches that key; the file
// is unreadable either way, and deleting it re-establishes the project.
//
// A file cut off mid-write ends with no closing quote on whatever string it
// was writing when it stopped. The check below does not confirm that case
// over a complete one: it takes whatever bytes follow the body's LAST quote
// character, truncated or not, and asks whether that is a non-empty prefix
// of the host. In a file that was never truncated, those trailing bytes are
// JSON punctuation -- a colon, a brace, a comma -- not the start of a real
// hostname, so running the same check there is loose but harmless.
func namesHost(body []byte, host string) bool {
	quoted := []byte(`"` + host + `"`)
	if bytes.Contains(body, quoted) {
		return true
	}
	i := bytes.LastIndexByte(body, '"')
	if i < 0 {
		return false
	}
	frag := string(body[i+1:])
	return frag != "" && len(frag) <= len(host) && strings.HasPrefix(host, frag)
}

// removeWhole deletes a baseline file this build will not rewrite. Called
// only once the caller has confirmed the file holds the host being forgotten,
// so a successful removal always counts as clearing it. A file that could not
// be removed is an error, never a quiet zero: the host is still in it.
func removeWhole(path string) (int, error) {
	if err := os.Remove(path); err != nil {
		return 0, err
	}
	return 1, nil
}
