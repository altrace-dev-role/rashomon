# DEL-7: budgets merge first and every entry has a row. Losing the floor row
# of an entry is the whole regression; the test enumerates the installer's
# events and the entries' floor classes.
m("DEL-7 the hook floor row leaves budgets.json", "test/perf/budgets.json",
  '    {"id": "hook.bash_short", "events": ["PreToolUse"], "command": ["hook"], "payload_class": "bash_short", "payload_bytes": 400, "benchmark": "BenchmarkRunHook/bash_short", "alloc_bytes_max": 49394, "maxrss_mib_p50": 6.5, "maxrss_mib_p95": 7.5, "parent_maxrss_mib_p50": null, "wall_ms_p50": 10, "wall_ms_p95": 20, "basis": "measured"},\n',
  '', "TestDEL7_")
m("DEL-7 a probe entry no longer names its event", "test/perf/budgets.json",
  '"events": ["SessionEnd"]', '"events": []', "TestDEL7_")
