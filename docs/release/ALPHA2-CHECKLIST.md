# v1.2.0-alpha.2: go/no-go checklist

This is the gate for the alpha.2 tag. It says what has to be true, who checks
it, the command that proves it, and where each row stands. Status is as of
2026-11-01. Update the status column in the PR that changes it.

The release notes draft and the mechanical tag steps are kept with the
maintainers' release card; this file is the gate they run behind.

## The rule

- Tag only when every P0 row passes.
- A P1 row may fail only if the release notes say so.
- "RC" means one frozen sha, the intended tag target. If main moves, the RC is
  a different sha and every row below that ran on the old one runs again.
- Every row except G6 and G13 runs on a binary built from the RC sha. G6 and
  G13 need the tag to exist, so they run after it.

Build the RC binary and run a live row like this, from a plain terminal and
never from an agent session (`ra-1.sh` refuses when `CLAUDECODE` is set):

    git checkout <rc-sha>
    go build -o /tmp/rc/rashomon ./cmd/rashomon
    RA_BIN=/tmp/rc/rashomon RA_ANY_VERSION=1 RA_EVIDENCE=<dir> test/live/ra-1.sh <ROW>

`ROW` is one of A1, A2, A4, A7, A8; the header of `test/live/ra-1.sh` says what
each one needs and saves. A3 is `test/live/p-tru-1.sh`. A5 and the live half of
G11 are not scripted yet; run them by hand and save the transcript and
`rashomon report --session` output.

Evidence for alpha.2 goes under one folder per row and is summarized in a
comment on the release-prep PR, one comment per row, so the maintainer can read
it without the machine it ran on.

## The gate

Owners are roles: Maintainer (Karthik: merges, CI), Reviewer (Luis: opens PRs,
runs the live rows, writes the notes), CI.

| # | Pri | Criterion | Owner | Proof | Status 2026-11-01 |
|---|---|---|---|---|---|
| G1 | P0 | No false red: a green `make`, `npm`, `yarn` or `pnpm` test run with another tool's "0 failed" line reads green. #180, #185 and #200 merged. | Maintainer merges, Reviewer reviews | `gh pr view 180 --json state` is MERGED (likewise 185, 200); `TestGZ1_`, `TestGZ1b_` and `TestMFL1_` green in CI | **FAIL.** #185 is merged into #180's branch. #200 (masked_run fixtures recorded from real unittest runs, #180's F1) is open. #180 is open. |
| G2 | P0 | `detach` restores a seeded `settings.json` byte for byte (A8) | Maintainer merges #189, Reviewer runs | `RA_BIN=... test/live/ra-1.sh A8` prints `A8 PASS`; the seeded and absent sha256s are equal before watch and after detach | **FAIL on alpha.1** (seeded `ba10ee42...` before watch, `616087f7...` after detach). #189 is open; the maintainer's F1/F2 review fixes are pushed at 70187559. |
| G3 | P0 | Full mutation sweep: 0 BUILD BROKEN, and every NOT DETECTED killed or recorded as equivalent | Maintainer merges #192, Reviewer reads the run | sweep run id on the RC sha, all shards green; `python3 test/mutation/sweep.py --strict --files <rows' files>` for a single row | **FAIL.** #192 is open. Full sweep run 41272115102 on #192's branch: every shard green except MO-1 (BUILD BROKEN), which #180's d3abd6ad fixes. |
| G4 | P0 | CI green on the RC sha: test with `-race`, fuzz, perf on linux and macOS, cross builds, pgraph, mutation shards | Maintainer | `gh run list --repo altrace-dev-role/rashomon --branch main --json conclusion,headSha,workflowName` shows success for every workflow on that sha | **NOT RUN.** There is no RC yet. The macOS perf job hits its 25-minute cap (the REC-34 mutants step) until #193 merges. Several perf rows on main sit at 96-101% of their gates (darwin cold `post.bash_short`; linux hook peak RSS), so a rerun may be needed. A re-measure is the maintainer's call. |
| G5 | P0 | Version strings say alpha.2: `plugin/.claude-plugin/plugin.json`, the default `RA_TAG` in `test/live/ra-1.sh` (lines 19 and 85), the README | Reviewer | `git show $tip:plugin/.claude-plugin/plugin.json \| grep version` | **FAIL.** `plugin.json` says 1.2.0-alpha.1. The release-prep PR is not opened; it waits for the PR set. |
| G6 | P0 | A1: `go install`, `install.sh` and the plugin zip all print `1.2.0-alpha.2`, sha256s recorded; and a real `/plugin` install through Claude Code | Reviewer | `RA_TAG=v1.2.0-alpha.2 test/live/ra-1.sh A1` writes `A1/install.txt`. Needs the tag, so it runs after it. | **NOT RUN** for alpha.2. alpha.1 failed only the `go install` route (it printed `dev`), which #166 fixes. |
| G7 | P0 | A2: the version floor is enforced. A spoofed 2.1.200 gives one notice and no note; the real version delivers the note | Reviewer (fixes the script, then runs it) | `ra-1.sh A2` prints `A2 PASS` | **INCONCLUSIVE on alpha.1.** The prompt was never typed, and the scenario cannot raise a note on a fresh store (no spec, no index). The `ra-1.sh` A2 fix is in progress. |
| G8 | P0 | A3: `p-tru-1.sh` exits 0 for PostToolUse and PostToolUseFailure on the current Claude Code, with the RC binary | Reviewer | `test/live/p-tru-1.sh`, exit 0, logs saved. On failure, add a floor or exclusion in `internal/steer/client.go` before the tag. | **NOT RUN on the RC.** Last pass was Claude Code 2.1.304, before the alpha.1 tag. |
| G9 | P0 | A4: the verified line after `go test`; the done line after an edit with no run | Reviewer | `ra-1.sh A4` prints `A4 PASS` | PASS on alpha.1 (Claude Code 2.1.308). Re-run on the RC. |
| G10 | P0 | A5: unpiped pytest gives the verified line; `pytest -q \| tail` is read from its summary with no false alarm; a failing piped run gives the failure line | Reviewer | transcripts plus `rashomon report --session` for each | **NOT RUN.** |
| G11 | P0 | No false red, live: a green `make test` delegating to `go test`, with a "0 failed"-style line in its output, reads green | Reviewer | `TestGZ1b_` in CI, plus one live session | **FAIL on alpha.1** (#180's body). Not run on the RC. |
| G12 | P0 | A9 and A10: the consent screen names only dispatched verbs (`TestTRU2c`); no `net` or `os/exec` imports and the README networking sentence is pinned (`TestTRU40`) | CI | green on the RC sha | PASS on the alpha.1 tag (CI run 41228999587). Re-run on the RC. |
| G13 | P0 | A11: the Release job is green; 8 assets; `isPrerelease: false`; `releases/latest` points to alpha.2 | Reviewer (tag), CI | `gh release view v1.2.0-alpha.2 --repo altrace-dev-role/rashomon --json isPrerelease,assets`; `curl -s https://api.github.com/repos/altrace-dev-role/rashomon/releases/latest \| grep tag_name` | **NOT RUN.** alpha.1 passed. Runs after the tag. |
| G14 | P0 | An agent session cannot switch oversight off: `detach`, `pause`, `steer off`, `purge`, `spec untrust`, `summary` and `bodies` off, and `consent <unit> off` all refuse under `CLAUDECODE=1` | Maintainer merges #184 | #184 merged; `TestAGRF1_` green; a live `!rashomon detach` inside Claude Code is refused | **Partial.** #176 and #177 are on main; #184 (`detach` and `pause`) is open. |
| G15 | P1 | A7: the report renders the session, names its gates, edits and notes, and its first screen is under 20 lines | Reviewer | `ra-1.sh A7` prints `A7 PASS` | PASS on alpha.1. Re-run on the RC (REPORT1a is now in). |
| G16 | P1 | README and consent screen are coherent on the RC: README3 plus the alpha.2 wording; acceptance README tests pass; no false sentence | Reviewer | `go test -count=1 ./test/acceptance -run 'TestReadme\|TestTRU40\|TestArguments\|TestH31'` and `go test ./test/prd`; `grep -n v1.1.0 README.md` hits only history lines | **FAIL.** Main's README names v1.1.0. #188 is open. |
| G17 | P1 | Release notes name every user-visible change and the known issues (see below) | Reviewer | the notes | **Draft.** |
| G18 | P1 | Rollback on paper: delete the release and the tag, and `latest` falls back to alpha.1; cut alpha.3 instead of reusing a tag | Reviewer | the commands in the release card | PASS (written). |
| G19 | P1 | A6: first-use value on unfamiliar repos. It does not block the tag; it decides the announcement wording | Reviewer | `evidence/first-use/<task>/` | **NOT RUN.** Task dirs are being built; the MEASURE1 go-humanize run (#103) was undiscriminated. |

Known issues the notes must name (G17): the consent screen asks again once on
an existing install (ScopeVersion 5); Windows is not usable; a downgrade to
alpha.1 cannot read an index written after #171; the agent-session guard reads
one environment variable and is not a security boundary.

## The alpha.2 PR set

Decided 2026-11-01 by Luis. "Merged" is as of the same date.

**Required** (no tag without these):

| PR | What it is | Gate row |
|---|---|---|
| #180 + #185 + #200 | the false red; #185 is merged into #180's branch; #200 holds the real-command fixtures | G1, G11 |
| #189 | A8: `detach` restores `settings.json` byte for byte | G2 |
| #192 | the mutant fixes | G3 |
| #184 | `detach` and `pause` refuse an agent session | G14 |
| #198 + #201 | the RES-5 fuzz oracle | G4 (fuzz) |

**In** (planned for alpha.2; named as pending in the notes if left out):
#179; #182 (merged); #183 + #186; #187 (merged); #178 + #181 (merged); and the
CI changes #193 (macOS job cap) and #199 (sweep shards).

**Everything else open is deferred** unless the release notes name it.

Before counting a PR as merged, check its base is `main` and CI is green on
that base. Several of these were opened against `pr/h16-load`, which is a dead
branch; a merge into it lands nowhere.

## Critical path

1. G1, G2, G3: the three P0 fixes.
2. The PR set merged to main.
3. Freeze the RC sha, and open the release-prep PR (G5, G16).
4. G4 on the frozen RC.
5. The live rows on the RC: G7, G8, G9, G10, G11, G14, and G15.
6. The owner says "tag".
7. After the tag: G6 and G13.

## Rollback

If anything fails after the tag, do not reuse the tag. The Go module proxy can
keep serving a deleted tag, and every tag becomes `latest` because
`.goreleaser.yml` has no release block. Delete the release and tag if you want
`latest` to fall back to alpha.1 (`gh release delete v1.2.0-alpha.2 --yes
--repo altrace-dev-role/rashomon`, then `git push origin
:refs/tags/v1.2.0-alpha.2`, then `git tag -d v1.2.0-alpha.2`), revert the README
commit if it merged, and cut alpha.3.

## Evidence index: alpha.1's live rows

Run 2026-11-01 on v1.2.0-alpha.1, macOS arm64, Claude Code 2.1.308 where a
session was involved. The raw files stay with the person who ran them; they
hold local paths and session ids. alpha.2's evidence is posted as a PR comment
per row.

| Row | Result | What the evidence shows |
|---|---|---|
| A1 install routes | FAIL | `install.sh` and the plugin zip both print `1.2.0-alpha.1`, and their darwin-arm64 binaries have the same sha256 (`20c3e811...`). `go install ...@v1.2.0-alpha.1` prints `dev`: the build info carries the module version but the binary did not report it. Fixed by #166. |
| A2 version floor | INCONCLUSIVE | No steer record for either session. Both captures show only `/exit`: the prompt was never typed. The scenario also could not raise a note on a fresh store. |
| A4 verified and done lines | PASS | Edit with no run: the done line at Stop and no verified line. Edit then `go test`: the verified line at Stop. Both read from the terminal capture or the person's word (the verdict says which). |
| A7 report | PASS | Both session reports carry the session id, "declarations recorded", "executions recorded" and "coverage"; each names one edit and one gate. |
| A8 detach | FAIL | Seeded `settings.json`: sha256 `ba10ee42...` before watch, `97b4d63e...` after watch, `616087f7...` after detach and after purge. Detach itself reported "could not restore settings.json byte for byte". The printed diff is in the hook arrays' layout. With no `settings.json` to start, detach and purge leave it absent, as wanted. |
| Release (A11) | PASS | CI on the tag commit, run 41228999587, success. Release workflow run 41230827276, success. 8 assets, `prerelease=false`, `releases/latest` is alpha.1. |

## Source notes

- This checklist lives in `docs/release/`; the repository's other docs are flat
  under `docs/`. It is a separate directory because it changes every release
  and is not engineering design.
- G14 is the agent-session guard row; G15 is A7.
