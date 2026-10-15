# DEL-18: a mutant whose target file's build constraint excludes the host
# GOOS is reported NOT JUDGED (platform), never run against a test that
# cannot see it and never counted undetected.
m("DEL-18 the sweep judges a platform-excluded mutant anyway", "test/mutation/sweep.py",
  "            if _goos_excludes(f, goos):\n", "            if False:\n", "TestDEL18_")
