test/live/autosteer: the live harness for AutoSteer's slices (PLAN.md section 7; AD13).

What it is. One script runs one `claude -p` session of a slice in a fresh scratch worktree of a pinned
commit of this repository, under one of three arms, and keeps every artifact the judges read. The judges
are scripts committed before the first run of a slice and never edited after one (a change opens a new
slice id). The agent never sees them.

Arms (run.sh ARM NAME):
  U  unaided: no rashomon hooks, no store.
  R  runtime: the main binary (RK_BIN_R) installed by `rashomon watch --yes` into the harness's Claude
     configuration directory, record scope consented; no graph verbs, no .rashomon in the tree.
  G  AutoSteer: the branch binary (RK_BIN_G) installed the same way, plus `pgraph sync`, `rashomon graph
     index` over t1/graph.json, and the knowledge import from the pinned files in RK_KNOWLEDGE.
Attribution: R minus U is the runtime's effect; G minus R is the graph intelligence's; G minus U the total.

Requirements. A Claude Code configuration directory that is logged in and holds no CLAUDE.md (RK_CONFIG_DIR;
run.sh refuses one with a CLAUDE.md, so neither arm loads instructions); `claude` on PATH (2.1.280 was the
version at pre-registration); the Go toolchain the tree pins (GOTOOLCHAIN=go1.27.3, offline in the module
cache); python3; perl (hook timing). Every `go` command the judges run goes through RK_GOSLOT when set
(the machine-wide Go slot script), else directly.

Layout.
  run.sh      one session, one arm, one run directory under RK_RUNS/NAME
  wrap.sh     installs the arm's hooks into RK_CONFIG_DIR/settings.json for the run and restores the file
  hook.sh     the hook entry the installed commands are prefixed with: logs verb, exit code, stdout, ms
  cleanup.sh  removes a run's scratch worktree by exact path (git worktree remove), never rm -rf a glob
  judge/common.py   the transcript, result and hook-log parsers every judge shares
  t1/         slice T1: prompt.txt (frozen), pins.txt (tree and binary shas, caps, sourced by the scripts), consumers.txt, graph.json
              (the G arm's index config), judge/ (the pre-registered checks A1 to A8 and judge.sh)

What a run directory holds. tree/ (the worktree at the pinned sha, kept until judged), store/ (R and G: the
install store, RASHOMON_HOME), result.json, transcript.jsonl (a copy of Claude Code's own), diff.patch,
status.txt, new-files.txt, report.txt (R and G), hooks.log and hook-ms.log (R and G), started, ended, exit,
session-id, arm, prompt.txt. The judges write judge/*.json and one evidence row.

What is never done here. No transcript content is printed by any script; the judges print counts, booleans,
file names and the brief's lines (which are the product's own output). No fixture, prompt, judge or engine
is changed after a run. No run is launched without the meter read and the slice's pre-registration commit on
the branch.

Facts verified while building the harness (2026-11-04, Claude Code 2.1.280, rashomon at 5f851724).
- `rashomon watch --yes` and `consent --yes` refuse when CLAUDECODE is set (Claude Code sets it for every command it
  runs). The scripts clear CLAUDECODE, CLAUDE_CODE_ENTRYPOINT and AI_AGENT for every child so a batch launched from a
  Claude Code session behaves as one launched from a terminal.
- watch writes the executable's real path into settings.json (/tmp is /private/tmp on macOS); wrap.sh matches both.
- pgraph pins GOTOOLCHAIN=local for its loader, so the `go` first on PATH must satisfy pgraph/go.mod (go 1.26.0);
  a 1.25 go on PATH fails the sync of this repository with "go.mod requires go >= 1.26.0". RK_GO_TOOLCHAIN_BIN names a
  bin dir such as the bin directory of the go1.27.3 toolchain module under $(go env GOMODCACHE)/golang.org/.
- The platform read appears on the pinned tree in the PostToolUse blast line (internal/hook/blastradius.go), the Stop's
  platform line (internal/recap/platform.go) and the prompt's symbols line (internal/intent/note.go); `rashomon report`
  has no platform section. A3 therefore reads the PostToolUse and Stop renderers and keeps the report as information.
- The symbols line today reads "direct callers N in M packages"; the A3 parser drops the "in M package(s)" count before
  it counts integers, so today's single-number line never parses as the two numbers.
- The Stop's platform row for a package is judged (and its "platform: go test ./<dir>/..." line printed) only when the
  pgraph facts are known, the package directory holds a _test.go, and a go test gate is declared by the project's own
  files (a CI step or a Makefile test target; go.mod alone stands for a convention and is excluded):
  verify.platformObligations. The A3 fixture copies therefore carry .github/workflows/ci.yml with `go test ./...` and
  an empty fixture_test.go per package; neither calls an oracle symbol.
- The Stop prints at most two package lines (recap.maxPlatformLines) and sends the rest to the ledger; A3 replays one
  symbol's file per session so no arm is read as silent for keeping the cap.
- The PostToolUse blast line is delivered as additionalContext only when steer.ClientVersion can read AI_AGENT in the
  form claude-code_<major>-<minor>-<patch>_harness; with any other form the note is built and not sent (one
  systemMessage per session says so). hook.sh exports that form from RK_CLAUDE_VERSION when Claude Code leaves the
  variable unset, and records what Claude Code did set in hook-env.txt once per run.
- The blast read needs the edit's cwd under a git root (blastRoot) and a prompt_id in the payload; the replay's fixture
  copies are git repositories and every payload carries prompt_id.
- Replayed on the pinned tree (smoke, main binary): the shape.go edit delivers "rashomon record: the platform graph
  names other packages with code calling into the package this call edited (at least 1): 'app'; test files are in 1 of
  them." at PostToolUse, and the Stop prints "platform: go test ./shape/... not run since the edit". Neither names a
  symbol or two numbers, so the pinned tree reads A3 neither, as pre-registered.
- pgraph is run by the shipped plugin's hooks.json (SessionStart and Stop, background), not by rashomon watch, so a
  watch-installed arm never syncs on its own. run.sh syncs R's tree with the pinned pgraph and G's with the arm's own
  pgraph before the session, so both arms carry the platform read the plugin gives a developer; U has no sidecars.

T1b (2026-11-05): slice T1 under the corrected harness. The pilot (nine runs, 06:53 to 10:41 PST, rows in the
execution record) found three defects in the harness and none in the product (ledger B448 to B450; held down by
test/acceptance/t1b_live_harness_test.go):
- B448: Claude Code 2.1.280 sets the entrypoint variable to sdk-cli for the hooks of a `claude -p` session (hook-env.txt of
  every R and G run); hook.sh exported cli only when the variable was unset, so every note was built for an sdk session
  and suppressed by design (no_mirror for the brief and the claim-vs-record line, unverified_channel for the blast and
  masked-status notes). hook.sh now exports cli always, after recording what Claude Code set. Reproduced and fixed by
  replay of the frozen prompt through the G binary before any paid run (zero bytes and a no_mirror row under sdk-cli;
  the brief delivered under cli), evidence under docs-out/plans/evidence-2026-11-05/b448-replay.
- B449: the batch driver's PATH held no go; a3's build, a6's vet and tests and a7's go test exited 127 on all nine runs.
  t1b/judge/judge.sh refuses (exit 3, no row) without go, gofmt, python3, perl and git on PATH; t1b/batch.sh puts
  RK_GO_TOOLCHAIN_BIN first on PATH and checks every tool before the first run.
- B450: a6 read gofmt's output alone, so a missing gofmt printed pass; t1b/judge/a6.sh reads the exit status too.
t1b/ holds its own pins.txt (the same tree, binaries, prompt, caps, knowledge digests and baseline log as T1), judge/
(T1's bytes with the two repairs and its own rows file t1b-rows.txt), batch.sh (the driver; RK_T1B_GO=yes only on the
owner's word; stops on a sanity, channel or judge failure), smoke.sh with smoke-prompt.txt (the channel smoke: one
haiku turn through the G hooks, a few cents, run before any paid run) and channel.py (the per-run channel check: what
Claude Code set recorded, no no_mirror or unverified_channel row, and for G the prompt hook's additionalContext and
its transcript attachment). run.sh takes RK_SLICE (t1 or t1b), RK_PROMPT_FILE and RK_MODEL; batch.sh clears the last
two so every batch run uses the frozen prompt and model, and the run directory records slice, model and prompt.
The T1 slice's files stay frozen at 158c7a29 (the freeze test in t1b_live_harness_test.go holds their digests); the T1 judge, re-run
unchanged over the nine pilot trees under a PATH with go, gives the pilot's A3 to A7 readings. The owner's word
(2026-11-05 15:5x PST, before any T1b run): the smoke and the n=3 batch; the turn cap raised to 150 turns and a 15 USD
cap in t1b/pins.txt (seven of nine pilot runs ended at 80 with partial trees, so the primary could not vary; A1a and
A1b stay comparable with the pilot); runs serial through one directory; the nine pilot trees kept for a blinded A3
read. The cost gate stays closed (ruling 12: every pilot arm's n=3 mean was above 2.50 USD), so n=10 waits for a word.
