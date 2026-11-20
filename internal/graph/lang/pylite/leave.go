package pylite

import (
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// envFile is the file PEP 405 puts at the top of a virtual environment.
const envFile = "pyvenv.cfg"

// LeftOut names a directory holding a regular file named pyvenv.cfg: a
// virtual environment (PEP 405), made by the venv module or a tool like
// it, whose site-packages and src hold other projects' modules. It stats
// that one name, without following a link, and reads no file, so a named
// pipe there cannot hold the walk; a link or a directory of that name is
// not an environment.
func (Resolver) LeftOut(root, rel string) (string, bool) {
	fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel), envFile))
	if err != nil || !fi.Mode().IsRegular() {
		return "", false
	}
	return lang.LeftEnvironment + " (holds " + envFile + ")", true
}
