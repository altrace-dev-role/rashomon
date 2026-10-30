#!/usr/bin/env python3
"""A stand-in for go, for measure1.sh's tests (card MEASURE1): version, env,
mod download, and test -json over a task whose one test passes when
calc.go holds the word FIXED and fails on its own assertion otherwise; a
calc.go holding BROKEN fails to build."""

import json
import os
import sys

args = sys.argv[1:]
if args[:1] == ["version"]:
    print("go version go1.25.6 stub")
elif args[:1] == ["env"]:
    for k in ("GOCACHE", "GOMODCACHE", "GOFLAGS", "GOPROXY", "GOTOOLCHAIN"):
        print("%s='%s'" % (k, os.environ.get(k, "")))
elif args[:2] == ["mod", "download"]:
    pass
elif args[:1] == ["test"]:
    pkg = "example.invalid/task"
    try:
        src = open("calc.go").read()
    except FileNotFoundError:
        src = ""
    have_test = os.path.exists("calc_test.go")
    if "BROKEN" in src:
        # go 1.24 and later: the build's own events name ImportPath; the
        # package's fail names the failed build.
        test_id = "%s [%s.test]" % (pkg, pkg)
        print(json.dumps({"ImportPath": test_id, "Action": "build-output", "Output": "./calc.go:3:1: syntax error\n"}))
        print(json.dumps({"ImportPath": test_id, "Action": "build-fail"}))
        print(json.dumps({"Action": "start", "Package": pkg}))
        print(json.dumps({"Action": "output", "Package": pkg, "Output": "FAIL\t%s [build failed]\n" % pkg}))
        print(json.dumps({"Action": "fail", "Package": pkg, "FailedBuild": test_id}))
        sys.exit(1)
    ok = "FIXED" in src or not have_test
    if have_test:
        print(json.dumps({"Action": "run", "Package": pkg, "Test": "TestAdd"}))
        print(json.dumps({"Action": "pass" if ok else "fail", "Package": pkg, "Test": "TestAdd"}))
    print(json.dumps({"Action": "pass" if ok else "fail", "Package": pkg}))
    sys.exit(0 if ok else 1)
else:
    print("go stub: %s is not stubbed" % " ".join(args), file=sys.stderr)
    sys.exit(2)
