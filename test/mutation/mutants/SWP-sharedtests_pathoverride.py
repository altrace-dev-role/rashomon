# --shared-tests lets a test point check() at a fixture instead of the real
# shared-tests.txt. Dropping the flag's wiring falls back to the real file
# silently, so a test that expects its fixture's entries to be read instead
# checks the project's actual file and the fixture's content is never seen.
m("the --shared-tests flag is silently ignored", "test/mutation/sweep.py",
  "    shared_tests = pathlib.Path(args.shared_tests) if args.shared_tests else SHARED_TESTS\n",
  "    shared_tests = SHARED_TESTS\n",
  "TestSharedTestsEntryShape")
