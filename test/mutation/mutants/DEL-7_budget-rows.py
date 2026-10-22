# DEL-7: budgets merge first and every entry has a row. Losing the floor row
# of an entry is the whole regression; the test enumerates the installer's
# events and the entries' floor classes.
m("DEL-7 the hook floor row leaves budgets.json", "test/perf/budgets.json",
  '    {"id": "hook.bash_short", "events": ["PreToolUse"], "command": ["hook"], "payload_class": "bash_short", "payload_bytes": 400, "benchmark": "BenchmarkRunHook/bash_short", "alloc_bytes_max": 49414, "maxrss_mib_p50": 5.88, "maxrss_mib_p95": 7.5, "parent_maxrss_mib_p50": null, "wall_ms_p50": 10, "wall_ms_p95": 20, "basis": "measured 2026-10-13 on the reference machine (go1.27.2, release flags; TestRES28b_ on a pinned, installed store): median of nine, the middle of three runs (5.88 / 5.91 / 5.88 MiB); expected per archive 85.4.1: 6.5 / 7.5 MiB, the floor of about 5.9 under the pin plus the masker, targets and written paths"},\n',
  '', "TestDEL7_")
m("DEL-7 a probe entry no longer names its event", "test/perf/budgets.json",
  '"events": ["SessionEnd"]', '"events": []', "TestDEL7_")
