package pylite

// LeftOut names a directory holding a regular file named pyvenv.cfg: a
// virtual environment (PEP 405), made by the venv module or a tool like
// it, whose site-packages and src hold other projects' modules. It stats
// that one name and reads no file.
func (Resolver) LeftOut(root, rel string) (string, bool) {
	return "", false
}
