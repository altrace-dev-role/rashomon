package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// FileConfig is the settings file a person's switches live in, beside the
	// anchors under the store root (`rashomon summary on|off` writes the
	// first key; later switches add theirs). Absent, every switch is at its
	// default, which for the summary is on (FUN-16, D71).
	FileConfig = "config.json"
	// maxConfigBytes bounds the file's read: a few switches.
	maxConfigBytes = 1 << 16
)

// Config is the switches' content. A nil switch is unset, which reads as its
// default, so a file written by a later build with a key this one does not
// know keeps it: WriteConfig puts back what ReadConfig found (extra).
type Config struct {
	// Summary is whether the Stop line prints its value line; nil is on.
	Summary *bool `json:"summary,omitempty"`

	extra map[string]json.RawMessage
}

// SummaryOn is the summary switch with its default applied: on unless the
// file says false.
func (c Config) SummaryOn() bool { return c.Summary == nil || *c.Summary }

// ReadConfig reads root's config. An absent file is the zero Config and no
// error, at the cost of one Lstat: the Stop hook calls this every turn, and
// most installs have none. A file that does not parse is an error, which the
// caller decides how to read.
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
		return Config{}, err
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
		return Config{}, fmt.Errorf("%s: %w", FileConfig, err)
	}
	delete(c.extra, "summary")
	return c, nil
}

// WriteConfig replaces root's config with c, keeping the keys ReadConfig
// found that this build does not name. There is no lock: the file holds
// switches a person sets by hand, one command at a time, and the replacement
// is atomic, so two writers cost the earlier one's change and never a torn
// file.
func WriteConfig(root string, c Config) error {
	out := make(map[string]json.RawMessage, len(c.extra)+1)
	for k, v := range c.extra {
		out[k] = v
	}
	if c.Summary != nil {
		b, err := json.Marshal(*c.Summary)
		if err != nil {
			return err
		}
		out["summary"] = b
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return ReplaceFile(filepath.Join(root, FileConfig), append(b, '\n'))
}
