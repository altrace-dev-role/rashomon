package consent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// SDK surface recording states harnesses.json carries (Technical spec 6.2,
// TD-15): the Agent SDK surface is paused until the consent screen's second
// question gets y.
const (
	SDKRecordPaused = "paused"
	SDKRecordOn     = "on"
)

// HarnessesPath is harnesses.json under a store root.
func HarnessesPath(root string) string { return filepath.Join(root, store.FileHarnesses) }

// WriteSDKAnswer records the answer to the consent screen's second question
// in root's harnesses.json as surfaces.sdk.record: "on" for y, "paused" for
// anything else. Every other key in the file is kept as it was read, and a
// surface's steer state is set to "off" only where none is recorded yet. The
// file is replaced by temporary file, sync and rename, at mode 0600.
//
// It records the answer only; what a paused surface records is the hook
// layer's to enforce.
func WriteSDKAnswer(root string, record bool) error {
	h, err := readHarnesses(root)
	if err != nil {
		return err
	}
	state := SDKRecordPaused
	if record {
		state = SDKRecordOn
	}
	return h.write(state)
}

// WriteSDKDefault is WriteSDKAnswer for a consent where the second question
// was not asked (--yes): it records the default, "paused", only where
// harnesses.json records no answer yet, and otherwise writes nothing, so an
// earlier y stays as it is.
func WriteSDKDefault(root string) error {
	h, err := readHarnesses(root)
	if err != nil {
		return err
	}
	if _, ok := h.sdk["record"]; ok {
		return nil
	}
	return h.write(SDKRecordPaused)
}

// harnesses is harnesses.json as read, down to surfaces.sdk, each level kept
// as raw JSON so a write changes only the keys it sets.
type harnesses struct {
	path               string
	doc, surfaces, sdk map[string]json.RawMessage
}

func readHarnesses(root string) (*harnesses, error) {
	h := &harnesses{
		path:     HarnessesPath(root),
		doc:      map[string]json.RawMessage{},
		surfaces: map[string]json.RawMessage{},
		sdk:      map[string]json.RawMessage{},
	}
	b, err := os.ReadFile(h.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		h.doc["version"] = json.RawMessage("1")
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &h.doc); err != nil || h.doc == nil {
			return nil, fmt.Errorf("consent: %s is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}
	if raw, ok := h.doc["surfaces"]; ok {
		if err := json.Unmarshal(raw, &h.surfaces); err != nil || h.surfaces == nil {
			return nil, fmt.Errorf("consent: %s surfaces is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}
	if raw, ok := h.surfaces["sdk"]; ok {
		if err := json.Unmarshal(raw, &h.sdk); err != nil || h.sdk == nil {
			return nil, fmt.Errorf("consent: %s surfaces.sdk is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}
	return h, nil
}

// write sets surfaces.sdk.record to state, and surfaces.sdk.steer to "off"
// where none is recorded, and replaces the file.
func (h *harnesses) write(state string) error {
	var err error
	if h.sdk["record"], err = json.Marshal(state); err != nil {
		return err
	}
	if _, ok := h.sdk["steer"]; !ok {
		h.sdk["steer"] = json.RawMessage(`"off"`)
	}
	if h.surfaces["sdk"], err = json.Marshal(h.sdk); err != nil {
		return err
	}
	if h.doc["surfaces"], err = json.Marshal(h.surfaces); err != nil {
		return err
	}
	out, err := json.Marshal(h.doc)
	if err != nil {
		return err
	}
	return replace(h.path, append(out, '\n'))
}
