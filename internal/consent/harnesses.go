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
	path := HarnessesPath(root)
	doc := map[string]json.RawMessage{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		doc["version"] = json.RawMessage("1")
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &doc); err != nil || doc == nil {
			return fmt.Errorf("consent: %s is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}

	surfaces := map[string]json.RawMessage{}
	if raw, ok := doc["surfaces"]; ok {
		if err := json.Unmarshal(raw, &surfaces); err != nil || surfaces == nil {
			return fmt.Errorf("consent: %s surfaces is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}
	sdk := map[string]json.RawMessage{}
	if raw, ok := surfaces["sdk"]; ok {
		if err := json.Unmarshal(raw, &sdk); err != nil || sdk == nil {
			return fmt.Errorf("consent: %s surfaces.sdk is not a JSON object; leaving it as it is", store.FileHarnesses)
		}
	}

	state := SDKRecordPaused
	if record {
		state = SDKRecordOn
	}
	if sdk["record"], err = json.Marshal(state); err != nil {
		return err
	}
	if _, ok := sdk["steer"]; !ok {
		sdk["steer"] = json.RawMessage(`"off"`)
	}
	if surfaces["sdk"], err = json.Marshal(sdk); err != nil {
		return err
	}
	if doc["surfaces"], err = json.Marshal(surfaces); err != nil {
		return err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return replace(path, append(out, '\n'))
}
