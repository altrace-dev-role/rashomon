package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	// FileConfig is the settings file a person's switches live in, beside the
	// anchors under the store root (`rashomon summary on|off` and `rashomon
	// steer off|on` write it). Absent, every switch is at its default, which
	// for the summary and for the notes is on (FUN-16, D71).
	FileConfig = "config.json"
	// FileConfigLock is the sidecar UpdateConfig locks: a file kept for the
	// lock alone, never config.json itself, which is replaced by rename under
	// the lock (FileHarnessCountsLock's reason).
	FileConfigLock = "config.lock"
	// configLockBudget is how long a switch's writer waits for another's.
	// Only a command a person typed waits; no hook writes the file.
	configLockBudget = 2 * time.Second
	// SwitchOff is the value a per-scope switch holds when it is off.
	SwitchOff = "off"
	// SwitchOn is the one value a steer switch that is set reads as on.
	SwitchOn = "on"
	// maxConfigBytes bounds the file's read: a few switches.
	maxConfigBytes = 1 << 16
)

// Config is the switches' content. A nil or empty switch is unset, which
// reads as its default, so a file written by a later build with a key this
// one does not know keeps it: UpdateConfig puts back what ReadConfig found
// (extra).
type Config struct {
	// Summary is whether the Stop line prints its value line for the user's
	// whole account; nil is on.
	Summary *bool `json:"summary,omitempty"`
	// SummaryProjects holds SwitchOff for each project (baseline.Key) whose
	// Stop line does not print its value line.
	SummaryProjects map[string]string `json:"summary_projects,omitempty"`
	// SteerUser is SwitchOff when the user has turned the notes off for every
	// project; "" is on.
	SteerUser string `json:"steer_user,omitempty"`
	// SteerProjects holds SwitchOff for each project (baseline.Key) whose
	// notes are off.
	SteerProjects map[string]string `json:"steer_projects,omitempty"`

	extra map[string]json.RawMessage
}

// configKeys are the keys Config names, which extra never holds.
var configKeys = []string{"summary", "summary_projects", "steer_user", "steer_projects"}

// SummaryOn is the summary switch for the user's account, with its default
// applied: on unless the file says false.
func (c Config) SummaryOn() bool { return c.Summary == nil || *c.Summary }

// SummaryOnFor is the summary switch as project reads it: off when the
// user's is off or the project's is.
func (c Config) SummaryOnFor(project string) bool {
	return c.SummaryOn() && c.SummaryProjects[project] != SwitchOff
}

// steerSwitchOff reads a steer switch's value: absent is on (the default),
// exactly "on" is on, and any other value is off, so a hand's "Off" or
// "disabled" clamps the notes rather than leaving them on. The writer only
// ever writes "off" (an on switch is absent).
func steerSwitchOff(v string) bool { return v != "" && v != SwitchOn }

// SteerOffUser reports the user's steer switch off.
func (c Config) SteerOffUser() bool { return steerSwitchOff(c.SteerUser) }

// SteerOffFor reports the notes off for project: the user's switch or the
// project's own.
func (c Config) SteerOffFor(project string) bool {
	return c.SteerOffUser() || steerSwitchOff(c.SteerProjects[project])
}

// ErrConfigUnreadable is what a config.json that is there but does not read
// (it cannot be opened, does not parse, or is over its size) matches under errors.Is. The hook
// clamps the notes on it (steer's config_unreadable) and `steer on --repair`
// is the one writer that replaces such a file.
var ErrConfigUnreadable = errors.New("config.json is unreadable")

// unreadableConfig is the read's error for a file that is there and does not
// read: the file's name and the reason, matching ErrConfigUnreadable.
type unreadableConfig struct{ err error }

func (e unreadableConfig) Error() string        { return FileConfig + ": " + e.err.Error() }
func (e unreadableConfig) Unwrap() error        { return e.err }
func (e unreadableConfig) Is(target error) bool { return target == ErrConfigUnreadable }

// ConfigReason is why err, a ReadConfig error, says the file does not read,
// without the file's name: a short reason a person can act on.
func ConfigReason(err error) string {
	var u unreadableConfig
	if errors.As(err, &u) {
		return u.err.Error()
	}
	return err.Error()
}

// ReadConfig reads root's config. An absent file is the zero Config and no
// error, at the cost of one Lstat: the Stop hook and every post call this,
// and most installs have none. A file that does not parse is an error, which
// the caller decides how to read.
func ReadConfig(root string) (Config, error) {
	path := filepath.Join(root, FileConfig)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, unreadableConfig{err}
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err == nil && len(b) > maxConfigBytes {
		err = fmt.Errorf("over %d bytes", maxConfigBytes)
	}
	var c Config
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err == nil {
		err = json.Unmarshal(b, &c.extra)
	}
	if err != nil {
		return Config{}, unreadableConfig{err}
	}
	for _, k := range configKeys {
		delete(c.extra, k)
	}
	return c, nil
}

// UpdateConfig is the one writer of root's config: under the lock on
// FileConfigLock it reads the file, applies change and replaces the file,
// keeping the keys ReadConfig found that this build does not name, and
// returns what it wrote. The lock is what lets two switches set at once both
// land (`summary off` beside `steer off`): the read-change-write is one hold,
// so neither's change is read before the other's is written. The replacement
// is atomic, so a reader never sees a torn file. A file that does not parse
// is an error and is not replaced.
func UpdateConfig(root string, change func(*Config)) (Config, error) {
	return updateConfig(root, change, false)
}

// RepairConfig is UpdateConfig for the one case it refuses: a file that is
// there and does not read (ErrConfigUnreadable) is replaced, its content
// lost, by the zero Config with change applied. A file that reads is updated
// as UpdateConfig does. Only a command a person confirmed calls it.
func RepairConfig(root string, change func(*Config)) (Config, error) {
	return updateConfig(root, change, true)
}

func updateConfig(root string, change func(*Config), repair bool) (Config, error) {
	lf, err := os.OpenFile(filepath.Join(root, FileConfigLock), os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return Config{}, err
	}
	defer lf.Close() //nolint:errcheck // nothing left to do with a close error once the lock is released
	unlock, _, err := lockCounted(lf, configLockBudget)
	if err != nil {
		return Config{}, err
	}
	defer unlock()
	c, err := ReadConfig(root)
	if err != nil && !(repair && errors.Is(err, ErrConfigUnreadable)) {
		return Config{}, err
	}
	change(&c)
	return c, writeConfig(root, c)
}

// writeConfig replaces root's config with c, the extra keys kept.
func writeConfig(root string, c Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	for k, v := range c.extra {
		if _, own := out[k]; !own {
			out[k] = v
		}
	}
	if b, err = json.Marshal(out); err != nil {
		return err
	}
	return ReplaceFile(filepath.Join(root, FileConfig), append(b, '\n'))
}
