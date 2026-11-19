# WS-1 (card WRITE-SAFE1): the index, the knowledge rows, a Python source and the
# settings file are written or read so a link cannot carry the write or the
# read out of the repository, a named pipe cannot hold a read, and a file over
# its bound is refused. One row per test's "Break:" line; where two tests name
# the same break (I1, I7 and I10b all save by path; I1k, I3 and I7k all write
# the rows by path; J10 and J11 both read by path) the row names every test.
# The guard rows (I10, I10k, J4, J13) break the other way: they refuse a link
# the fix must follow. The mode row mirrors SRR-1's: a named pipe opened
# without blocking reads as empty, not as refused.
GRAPH_RET = "\treturn rootio.ReplaceFile(r, filepath.FromSlash(GraphFile), 0o600, func(w io.Writer) error { return Save(w, s, h) })\n"
m("WS-1 the index is saved by path through a linked directory", "internal/graph/persist.go",
  GRAPH_RET,
  "\tpath := filepath.Join(root, filepath.FromSlash(GraphFile))\n"
  "\tif err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {\n\t\treturn err\n\t}\n"
  "\tf, err := os.CreateTemp(filepath.Dir(path), \"graph-*.tmp\")\n\tif err != nil {\n\t\treturn err\n\t}\n"
  "\tif err := Save(f, s, h); err != nil {\n\t\tf.Close()\n\t\treturn err\n\t}\n\tf.Close()\n"
  "\t_ = rootio.ReplaceFile\n\treturn os.Rename(f.Name(), path)\n",
  "TestWS1_I1_|TestWS1_I7_|TestWS1_I10b_")
m("WS-1 the index is written by opening graph.jsonl", "internal/graph/persist.go",
  GRAPH_RET,
  "\t_ = rootio.ReplaceFile\n"
  "\tif err := r.MkdirAll(filepath.Dir(filepath.FromSlash(GraphFile)), 0o755); err != nil {\n\t\treturn err\n\t}\n"
  "\tf, err := os.Create(filepath.Join(root, filepath.FromSlash(GraphFile)))\n\tif err != nil {\n\t\treturn err\n\t}\n"
  "\tdefer f.Close()\n\treturn Save(f, s, h)\n",
  "TestWS1_I5_")
m("WS-1 every link at .rashomon is refused for the index", "internal/graph/persist.go",
  GRAPH_RET,
  "\tif fi, err := os.Lstat(filepath.Join(root, \".rashomon\")); err == nil && fi.Mode()&os.ModeSymlink != 0 {\n\t\treturn os.ErrPermission\n\t}\n" + GRAPH_RET,
  "TestWS1_I10_")
ROWS_RET = "\treturn filepath.Join(root, rel), rootio.ReplaceFile(r, rel, 0o644, func(w io.Writer) error {\n\t\t_, err := w.Write(b)\n\t\treturn err\n\t})\n"
m("WS-1 knowledge rows are written by path through a linked directory", "internal/graph/knowledge/schema.go",
  ROWS_RET,
  "\t_ = rootio.ReplaceFile\n\t_ = io.EOF\n\tp := filepath.Join(root, rel)\n"
  "\tif err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {\n\t\treturn \"\", err\n\t}\n"
  "\treturn p, os.WriteFile(p, b, 0o644)\n",
  "TestWS1_I1k_|TestWS1_I3_|TestWS1_I7k_")
m("WS-1 a knowledge row file is written by opening name.json", "internal/graph/knowledge/schema.go",
  ROWS_RET,
  "\t_ = rootio.ReplaceFile\n\t_ = io.EOF\n"
  "\tif err := r.MkdirAll(filepath.Dir(rel), 0o755); err != nil {\n\t\treturn \"\", err\n\t}\n"
  "\treturn filepath.Join(root, rel), os.WriteFile(filepath.Join(root, rel), b, 0o644)\n",
  "TestWS1_I4_")
m("WS-1 every link at the knowledge directory is refused", "internal/graph/knowledge/schema.go",
  ROWS_RET,
  "\tif fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(Dir))); err == nil && fi.Mode()&os.ModeSymlink != 0 {\n\t\treturn \"\", os.ErrPermission\n\t}\n" + ROWS_RET,
  "TestWS1_I10k_")
LOAD = "\tdata, err := regfile.ReadFile(path, maxFileBytes)\n"
SETTINGS_IMPORTS = ("internal/settings/document.go", "\t\"io/fs\"\n\t\"strings\"\n", "\t\"io/fs\"\n\t\"os\"\n\t\"strings\"\n")
m("WS-1 a settings file that is a link is refused", "internal/settings/document.go",
  LOAD,
  "\tif fi, lerr := os.Lstat(path); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {\n\t\treturn nil, fs.ErrPermission\n\t}\n" + LOAD,
  "TestWS1_J4_")
IMPORTS["WS-1 a settings file that is a link is refused"] = SETTINGS_IMPORTS
m("WS-1 a settings file is read whole whatever its size", "internal/settings/document.go",
  LOAD, "\tdata, err := regfile.ReadFile(path, 1<<62)\n", "TestWS1_J5_")
m("WS-1 settings opens by path with a blocking open", "internal/settings/document.go",
  LOAD, "\tdata, err := os.ReadFile(path)\n", "TestWS1_J3_")
IMPORTS["WS-1 settings opens by path with a blocking open"] = SETTINGS_IMPORTS
m("WS-1 a settings syntax error returns the scanner's text", "internal/settings/errors.go",
  "\t\t\t\tif strings.HasPrefix(syn.Error(), \"invalid character\") {\n",
  "\t\t\t\tif false && strings.HasPrefix(syn.Error(), \"invalid character\") {\n", "TestWS1_J6_")
SRC = "\t\tb, err := rootio.ReadRegular(rt, filepath.FromSlash(rel), maxSourceBytes)\n"
m("WS-1 a Python source is read by path", "internal/graph/lang/pylite/pylite.go",
  SRC, "\t\tb, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))\n\t\t_ = rootio.ReadRegular\n",
  "TestWS1_J10_|TestWS1_J11_")
m("WS-1 a Python source is read whole whatever its size", "internal/graph/lang/pylite/pylite.go",
  SRC, "\t\tb, err := rootio.ReadRegular(rt, filepath.FromSlash(rel), 1<<62)\n", "TestWS1_J12_")
m("WS-1 every Python source that is a link is refused", "internal/graph/lang/pylite/pylite.go",
  SRC,
  "\t\tif fi, lerr := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {\n\t\t\treturn nil, os.ErrPermission\n\t\t}\n" + SRC,
  "TestWS1_J13_")
m("WS-1 the rooted read waits on a named pipe for a writer", "internal/rootio/rootio.go",
  "os.O_RDONLY|syscall.O_NONBLOCK, 0)", "os.O_RDONLY|syscall.O_NONBLOCK&0, 0)", "TestWS1_J11_")
m("WS-1 the rooted read reads what is not a regular file", "internal/rootio/rootio.go",
  "\tif !fi.Mode().IsRegular() {\n", "\tif fi == nil {\n", "TestWS1_J11_")
