package index

import (
	"errors"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// Inferred is the configuration Infer made for a root with none: the
// configuration, the bytes of its file, each unit with its language and
// the manifest and rule it came from, and each path left out with why.
type Inferred struct {
	Config Config
	Bytes  []byte
	Units  []InferredUnit
	Left   []lang.Left
}

// InferredUnit is one inferred unit and its language.
type InferredUnit struct {
	Lang string
	lang.Unit
}

// Infer is the configuration of root inferred by each language Wire
// registers that is a lang.Inferrer, in Wire's order, over one walk of root
// that skips what Build's walk skips and every directory below root that
// holds a configuration of its own (another index's root, left out and
// named). Its file is encoded as the configuration file is read, one unit
// to a line. No unit is ErrNoConfig, the error naming the manifests looked
// for; a file over MaxConfigBytes is an error naming the cap.
func Infer(root string) (Inferred, error) {
	return Inferred{}, errors.New("index: Infer is not built yet")
}
