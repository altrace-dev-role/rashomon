# PW1b-ON-230 1b: the plugin runs pgraph sync -hook in the background at
# session start and at the Stop, never per tool call, through a launcher that
# prints nothing and exits 0 without a build for this platform, and says so
# where the person consents. The background row's own gate (perf tag) is
# judged in the perf run, not here: the sweep runs no tags.
HJ = "plugin/hooks/hooks.json"
L = "plugin/bin/pgraph"
STOPG = """      {
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/bin/pgraph",
            "args": ["sync", "-hook"],
            "timeout": 90,
            "async": true
          }
        ]
      }
    ],
    "StopFailure\""""

m("PW1b the Stop entry blocks the reply", HJ,
  STOPG, STOPG.replace('"async": true', '"async": false'), "TestPW1b_ThePluginRuns")
m("PW1b the Stop entry gets a matcher", HJ,
  STOPG, STOPG.replace('"hooks": [', '"matcher": "*", "hooks": [', 1), "TestPW1b_ThePluginRuns")
m("PW1b the launcher prints on stdout", L,
  'exec "$bin" "$@"', 'echo pgraph\nexec "$bin" "$@"', "TestPW1b_ThePluginLauncher")
m("PW1b the launcher fails without a binary", L,
  '\techo "pgraph: this plugin has no build for $os-$arch, so no graph is written" >&2\n\texit 0',
  '\techo "pgraph: this plugin has no build for $os-$arch, so no graph is written" >&2\n\texit 1',
  "TestPW1b_ThePluginLauncher")
m("PW1b the build makes no pgraph", "scripts/build-plugin.sh",
  '-o "../plugin/bin/pgraph-$os-$arch" ./cmd/pgraph', '-o "../plugin/bin/pgraph-x-$os-$arch" ./cmd/pgraph',
  "TestPW1b_ThePluginRuns")
m("PW1b the built pgraph is committable", ".gitignore",
  "/plugin/bin/pgraph-*", "", "TestPW1b_ThePluginRuns")
m("PW1b the screen keeps the per-call words", "internal/disclose/paragraphs.go",
  "at session start and each time Claude finishes a reply, never per tool call.",
  "at session start and after each Write, Edit, MultiEdit, NotebookEdit or Bash call.",
  "TestPW1b_TheWriterIs")
m("PW1b the description drifts from the manifests", "internal/disclose/plugin.go",
  '"background at session start and each time Claude finishes a reply: the go tool',
  '"background after each edit or command: the go tool', "TestPW1b_TheWriterIs|TestPC1c_")
m("PW1b a perf run owes no background gate", "test/perf/runs.go",
  "\t\towed = append(owed, BackgroundGate(r.ID))", "\t\t_ = r", "TestPW1b_APerfRun")
m("PW1b the background row gates everywhere", "test/perf/gate.go",
  '\tif plat == referencePlatform {\n\t\treturn r.CurrentMaxRSSMiBP50, true, ""',
  '\tif true {\n\t\treturn r.CurrentMaxRSSMiBP50, true, ""', "TestPW1b_ABackgroundRow")
m("PW1b the row forgets the Stop", "test/perf/budgets.json",
  '"events": ["SessionStart", "Stop"], "command": ["pgraph", "sync", "-hook"]',
  '"events": ["SessionStart"], "command": ["pgraph", "sync", "-hook"]', "TestPW1b_EveryAsync")
