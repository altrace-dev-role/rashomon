# A row judged by a nested module's own tests (pgraph/, which has a go.mod of
# its own) printed NOT DETECTED, because `go test ./...` from the root stops at
# that module, and --check, reading every *_test.go under the root, passed it
# (SWEEP-MODULE1). A file now declares MODULE = "<dir>": --check finds each
# row's test among its module's tests alone, read as ./... reads them, and the
# sweep runs the row there behind that module's own baseline. Each row drops
# one of those pieces.
S = "test/mutation/sweep.py"
C = "TestSWM1_CheckRefusesATestOutsideTheModuleThatRunsIt"
R = "TestSWM1b_AModuleRowRunsInItsModuleBehindItsOwnBaseline"
m("SWEEP-MODULE1 a file's MODULE is ignored", S,
  '        module = ns.get("MODULE", ".")\n', '        module = "."\n', C)
m("SWEEP-MODULE1 a MODULE that is not a string is taken as a directory", S,
  "        if isinstance(module, str) and module:\n", "        if True:\n", C)
m("SWEEP-MODULE1 --check reads tests under testdata", S,
  '            if x.startswith((".", "_")) or x in ("testdata", "vendor"):\n',
  '            if x.startswith((".", "_")) or x in ("vendor",):\n', C)
m("SWEEP-MODULE1 --check reads a nested module's tests as the root's", S,
  '            if os.path.isfile(os.path.join(d, x, "go.mod")):\n', "            if False:\n", C)
m("SWEEP-MODULE1 --check matches a row's test against every module's tests", S,
  "            if own is not None and not any(rx.search(n) for n in own):\n",
  "            if own is not None and not any(rx.search(n) for ns in go_tests.values() for n in ns):\n", C)
m("SWEEP-MODULE1 a MODULE with no go.mod is not refused as one", S,
  '        elif not os.path.isfile(os.path.join(module, "go.mod")):\n', "        elif False:\n", C)
m("SWEEP-MODULE1 a MODULE outside the current directory is not refused as one", S,
  "        if outside_cwd(module):\n", "        if False:\n", C)
m("SWEEP-MODULE1 a module row runs in the root", S,
  '"-count=1", "-v"], cwd=MODULE_OF.get(name, "."))\n', '"-count=1", "-v"], cwd=".")\n', R)
m("SWEEP-MODULE1 only the root's baseline runs", S,
  '    for module in sorted({MODULE_OF.get(x[0], ".") for x in M}) or ["."]:\n', '    for module in ["."]:\n', R)
m("SWEEP-MODULE1 a module's baseline runs in the root", S,
  '    _baseline = run_go_test(["go", "test", "./...", "-count=1"], cwd=module)\n',
  '    _baseline = run_go_test(["go", "test", "./...", "-count=1"], cwd=".")\n', R)
m("SWEEP-MODULE1 go test runs in the sweep's own directory whatever cwd it is given", S,
  "    r = subprocess.run(argv, capture_output=True, cwd=cwd)\n", "    r = subprocess.run(argv, capture_output=True)\n", R)
