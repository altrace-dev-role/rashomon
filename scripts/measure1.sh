#!/usr/bin/env bash
# measure1.sh -- the paired run's controller (card MEASURE1): one task, two
# interactive Claude Code sessions that differ in one declared input,
# RASHOMON_STEER_ARM (off: every note decided and withheld; steer: delivered),
# then the oracle on each, the sensors, and `rashomon measure pair`.
#
#   measure1.sh prepare BIN [DIR]
#       make M (DIR, or a new temp directory), copy BIN, mark M/tpl/store a
#       lab, and print the owner's two one-time commands.
#   measure1.sh run M --task DIR [--model ID] [--order seed:N] [--reps K]
#       [--limit SECONDS] [--allowed TOOLS]
#       the gates G0, G2 (the oracle, before any session spends a token),
#       G1, G3 to G6; each stops the run on failure.
#   measure1.sh task M --task DIR
#       G2 alone: whether DIR is a task at all (its FAIL_TO_PASS red on base
#       on their own assertions, everything green on the reference), with
#       no session started.
#
# A task directory holds repo/ (a git repository), base and fix (one commit
# id each), issue.txt, optionally prompts.txt (one prompt per paragraph;
# default three), FAIL_TO_PASS ("<package> <Test>" per line) and PASS_TO_PASS
# (one package per line).
#
# The driver runs Claude Code and go; the binary under test only records and
# reads. MEASURE1_CLAUDE, MEASURE1_TMUX and MEASURE1_POLICY_DIR replace the
# pinned claude, tmux and the managed-policy directory, for the driver's own
# tests.
set -euo pipefail

PIN=2.1.280
PREFIX="Task: "
here=$(cd "$(dirname "$0")" && pwd)
claude=${MEASURE1_CLAUDE:-$HOME/.local/share/claude/versions/$PIN}
tmux=$(command -v "${MEASURE1_TMUX:-tmux}" || true)
policy=${MEASURE1_POLICY_DIR:-/Library/Application Support/ClaudeCode}

check() { python3 "$here/measure1_check.py" "$@"; }
say() { printf 'measure1: %s\n' "$*"; }
die() { say "$*" >&2; exit 1; }
now() { date +%s; }
fsize() { if [ -f "$1" ]; then wc -c <"$1" | tr -d ' '; else echo 0; fi; }
hex8() { od -An -N4 -tx1 /dev/urandom | tr -d ' \n'; }
put() { printf -v "$1" '%s' "$2"; }
# A deviation goes to the log and to stderr, never stdout: turn's caller
# captures stdout as the turn's end.
deviation() { printf '%s %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$*" >>"$M/out/deviations.log"; say "deviation: $*" >&2; }

prepare() {
	local bin=$1
	M=${2:-$(mktemp -d "${TMPDIR:-/tmp}/measure1.XXXXXX")}
	case $M in /*) ;; *) M=$PWD/$M ;; esac
	mkdir -p "$M/bin" "$M/tpl/cc" "$M/tpl/store" "$M/out" "$M/modcache"
	cp "$bin" "$M/bin/rashomon"
	chmod 755 "$M/bin/rashomon"
	ln -sf "$(command -v go)" "$M/bin/go"
	ln -sf "$(command -v git)" "$M/bin/git"
	printf '[user]\n\tname = measure1\n\temail = measure1@example.invalid\n' >"$M/gitconfig"
	"$M/bin/rashomon" measure --prepare --home "$M/tpl/store"
	cat <<EOF
measure1: M=$M
measure1: the owner's one-time act, at a terminal (never inside an agent session):
  1. CLAUDE_CONFIG_DIR="$M/tpl/cc" "$claude"
     then /login, then /exit
  2. CLAUDE_CONFIG_DIR="$M/tpl/cc" RASHOMON_HOME="$M/tpl/store" "$M/bin/rashomon" watch
     and answer y at the terminal
measure1: then: $0 run "$M" --task DIR
EOF
}

# armenv fills E with the arm's whole environment (G1): nothing of the
# driver's own passes but what is named here.
armenv() {
	E=(env -i "HOME=$HOME" "USER=${USER:-}" "LOGNAME=${LOGNAME:-${USER:-}}" "SHELL=/bin/sh"
		"TERM=xterm-256color" "LANG=${LANG:-en_US.UTF-8}" "PATH=$M/bin:/usr/bin:/bin:/usr/sbin:/sbin"
		"TMPDIR=$1/tmp" "CLAUDE_CODE_TMPDIR=$1/tmp" "CLAUDE_CONFIG_DIR=$1/cc"
		"CLAUDE_SECURESTORAGE_CONFIG_DIR=$M/tpl/cc" "RASHOMON_HOME=$1/store" "RASHOMON_STEER_ARM=$2"
		"DISABLE_AUTOUPDATER=1" "GOTOOLCHAIN=local" "GOENV=off" "GOMODCACHE=$M/modcache"
		"GOFLAGS=-mod=readonly" "GOPROXY=off" "GOCACHE=$1/gocache"
		"GIT_CONFIG_GLOBAL=$M/gitconfig" "GIT_CONFIG_NOSYSTEM=1")
}

# newarm makes a fresh arm root A (its own temp directory) with
# pristine copies of the templates and a workspace from the base tree only.
newarm() {
	A=$(mktemp -d "${TMPDIR:-/tmp}/measure1-arm.XXXXXX")
	mkdir -p "$A/tmp" "$A/gocache"
	chmod 700 "$A/tmp"
	cp -R "$M/tpl/cc" "$A/cc"
	cp -R "$M/tpl/store" "$A/store"
	WS="$A/ws-$(hex8)"
	mkdir "$WS"
	git -C "$task/repo" archive "$base" | tar -x -C "$WS"
	(cd "$WS" && GIT_CONFIG_GLOBAL="$M/gitconfig" GIT_CONFIG_NOSYSTEM=1 git init -q &&
		GIT_CONFIG_GLOBAL="$M/gitconfig" GIT_CONFIG_NOSYSTEM=1 git add -A &&
		GIT_CONFIG_GLOBAL="$M/gitconfig" GIT_CONFIG_NOSYSTEM=1 GIT_AUTHOR_DATE=2000-01-01T00:00:00Z \
			GIT_COMMITTER_DATE=2000-01-01T00:00:00Z git commit -q -m base)
	check trust "$A/cc" "$WS"
	ARMS+=("$A")
}

# tarball keeps an arm's workspace, Claude Code directory and store in M/out
# and removes its root, before anything else starts.
tarball() {
	local a=$1 name=$2
	[ -d "$a" ] || return 0
	if [ -S "$a/tmux.sock" ]; then "$tmux" -S "$a/tmux.sock" kill-server 2>/dev/null || true; fi
	tar -C "$a" -cf "$M/out/$name.tar" . 2>/dev/null || deviation "tar of $name incomplete"
	rm -rf "$a"
}

cleanup() {
	local a
	for a in "${ARMS[@]:-}"; do
		[ -n "$a" ] || continue
		if [ -f "$a/label" ]; then tarball "$a" "aborted-$(cat "$a/label")"; else tarball "$a" "aborted-$(basename "$a")"; fi
	done
}

# launch starts the arm's claude in a private tmux server: the server
# itself under the allowlist, the client exec'd with no shell between.
launch() {
	local a=$1 arm=$2 sid=$3
	armenv "$a" "$arm"
	"${E[@]}" "$tmux" -S "$a/tmux.sock" -f /dev/null new-session -d -s arm -x 200 -y 50 -c "$WS" \
		"${E[@]}" "$claude" --session-id "$sid" --model "$model" --settings "$a/cc/settings.json" \
		--setting-sources project,local --strict-mcp-config --permission-mode acceptEdits --allowedTools "$allowed"
	local t0 cov="$a/store/runs/$sid/coverage.ndjson"
	t0=$(now)
	until [ -f "$cov" ] && grep -Eq '"phase": *"start"' "$cov"; do
		[ $(($(now) - t0)) -lt 60 ] || return 1
		sleep 1
	done
}

# gone is true once the arm's client has exited: the private server ends
# with its only session.
gone() { ! "$tmux" -S "$1/tmux.sock" has-session -t arm 2>/dev/null; }

send() {
	"$tmux" -S "$1/tmux.sock" load-buffer -b p "$2"
	"$tmux" -S "$1/tmux.sock" paste-buffer -p -d -b p -t arm
	sleep 1
	"$tmux" -S "$1/tmux.sock" send-keys -t arm Enter
}

# turn runs prompt i to its end: confirmed from the transcript, ended
# by the first stop checkpoint past the records offset, watched by an
# inactivity timer. It prints the end type, or "blocked (why)".
turn() {
	local a=$1 sid=$2 pf=$3 rec="$1/store/runs/$2/records.ndjson" tr roff toff t0 rc sent=1 last end
	tr=$(check transcript "$a/cc" "$sid")
	roff=$(fsize "$rec")
	toff=$(fsize "$tr")
	send "$a" "$pf"
	t0=$(now)
	while :; do
		tr=$(check transcript "$a/cc" "$sid")
		rc=1
		if [ -n "$tr" ]; then check submitted "$tr" "$toff" "$pf" >/dev/null && rc=0 || rc=$?; fi
		[ "$rc" = 0 ] && break
		[ "$rc" = 3 ] && { echo "blocked (submission unconfirmable)"; return; }
		gone "$a" && { echo "blocked (client exited)"; return; }
		if [ $(($(now) - t0)) -ge 30 ]; then
			[ "$sent" = 1 ] || { echo "blocked (not submitted)"; return; }
			deviation "prompt $(basename "$pf") not seen in the transcript after 30 s; sent once more"
			send "$a" "$pf"
			sent=2
			t0=$(now)
		fi
		sleep 1
	done
	last=$(fsize "$rec")
	t0=$(now)
	while :; do
		if end=$(check turnend "$rec" "$roff" "$tr" "$toff"); then
			echo "$end"
			return
		fi
		if gone "$a"; then
			# a stop written just before the exit still ends the turn
			check turnend "$rec" "$roff" "$tr" "$toff" || echo "blocked (client exited)"
			return
		fi
		if [ "$(fsize "$rec")" != "$last" ]; then
			last=$(fsize "$rec")
			t0=$(now)
		elif [ $(($(now) - t0)) -ge "$limit" ]; then
			echo "blocked ($(check pending "$rec"))"
			return
		fi
		sleep 2
	done
}

# session runs one arm's session over the first n prompts and records each
# turn's end in M/out/<label>.turns.
session() {
	local a=$1 arm=$2 n=$3 label=$4 sid i end
	sid=$(check uuid)
	echo "$sid" >"$M/out/$label.session"
	echo "$label" >"$a/label"
	: >"$M/out/$label.turns"
	launch "$a" "$arm" "$sid" || { echo "blocked (no SessionStart record in 60 s)" >>"$M/out/$label.turns"; return; }
	for i in $(seq 1 "$n"); do
		end=$(turn "$a" "$sid" "$M/prompt-$i.txt")
		echo "prompt $i: $end" >>"$M/out/$label.turns"
		case $end in blocked*)
			"$tmux" -S "$a/tmux.sock" capture-pane -p -t arm >"$M/out/$label.pane-$i.txt" 2>/dev/null || true
			break ;;
		esac
	done
	"$tmux" -S "$a/tmux.sock" send-keys -t arm -l "/exit" 2>/dev/null || true
	"$tmux" -S "$a/tmux.sock" send-keys -t arm Enter 2>/dev/null || true
	local t0
	t0=$(now)
	while "$tmux" -S "$a/tmux.sock" has-session -t arm 2>/dev/null && [ $(($(now) - t0)) -lt 30 ]; do sleep 1; done
	"$tmux" -S "$a/tmux.sock" kill-server 2>/dev/null || true
}

g0() {
	case $claude in /*) ;; *) die "G0: claude must be an absolute path, not $claude" ;; esac
	[ -n "$tmux" ] || die "G0: no tmux"
	CLAUDE_V=$("$claude" --version | awk '{print $1}')
	[ "$CLAUDE_V" = "$PIN" ] || die "G0: claude is $CLAUDE_V at $claude, not the pinned $PIN"
	GO_V=$("$M/bin/go" version)
	BIN_SHA=$(shasum -a 256 "$M/bin/rashomon" | awk '{print $1}')
	local f
	for f in managed-settings.json managed-mcp.json CLAUDE.md; do
		[ ! -e "$policy/$f" ] || die "G0: the managed policy file $policy/$f exists"
	done
	[ -f "$M/tpl/store/lab" ] || die "G0: $M/tpl/store is not a lab store: run prepare"
	[ -f "$M/tpl/cc/settings.json" ] || die "G0: $M/tpl/cc has no settings.json: the owner's act has not run"
	git -C "$here/.." grep -n -e 'EnvSteerArm' -e 'armOff(' -e 'systemMessage' -- cmd internal \
		>"$M/out/arm-sites.txt" 2>/dev/null || deviation "arm-read sites not listed: $here/.. is not a git tree"
	check prereg "$M/out/prereg.json" "claude=$CLAUDE_V" "claude_path=$claude" "go=$GO_V" "rashomon_sha256=$BIN_SHA" \
		"model=$model" "order=$order" "reps=$reps" "limit=$limit" "allowed=$allowed" "base=$base" "fix=$fix" \
		"tree=$(git -C "$here/.." rev-parse HEAD 2>/dev/null || echo unknown)" "at=$(date '+%Y-%m-%dT%H:%M:%S%z')" \
		-- "$task/issue.txt" "$task/FAIL_TO_PASS" "$task/PASS_TO_PASS" "$M"/prompt-*.txt "$M/out/arm-sites.txt"
	say "G0 claude $CLAUDE_V at $claude (pinned); $GO_V; rashomon $BIN_SHA; no managed policy"
	local dropped
	dropped=$(env | grep -E -o '^(ANTHROPIC_[A-Z_]*|CLAUDE[A-Z_]*|RASHOMON_[A-Z_]*|[A-Za-z_]*_PROXY)=' | tr -d '=' | sort | tr '\n' ' ' || true)
	say "G1 dropped from every arm: ${dropped:-nothing}"
}

prompts() {
	local i=1 text
	if [ -f "$task/prompts.txt" ]; then
		awk -v RS= -v pre="$PREFIX" -v dir="$M" '{ f = dir "/prompt-" NR ".txt"; printf "%s%s", pre, $0 > f; close(f) }' "$task/prompts.txt"
	else
		printf '%s%s' "$PREFIX" "$(cat "$task/issue.txt")" >"$M/prompt-1.txt"
		printf '%s%s' "$PREFIX" "Run the project's tests and fix what fails." >"$M/prompt-2.txt"
		printf '%s%s' "$PREFIX" "Summarize what you changed." >"$M/prompt-3.txt"
	fi
	NPROMPTS=$(find "$M" -maxdepth 1 -name 'prompt-*.txt' | wc -l | tr -d ' ')
}

# g1 runs the probes in scratch arms: G1b's -p init event, G1c's interactive
# first prompt with G1a's hook-side dump, G1d's CLAUDE.md walk with its
# controls, G1e's repository files.
g1() {
	local arm a sid text tr plant decoy
	for arm in off steer; do
		newarm
		put "P1_$arm" "$A"
		armenv "$A" "$arm"
		sid=$(check uuid)
		(cd "$WS" && "${E[@]}" "$claude" -p "${PREFIX}reply with the word ok" --output-format stream-json --verbose \
			--max-turns 1 --session-id "$sid" --model "$model" --settings "$A/cc/settings.json" \
			--setting-sources project,local --strict-mcp-config --permission-mode acceptEdits \
			--allowedTools "$allowed" >"$M/out/g1b-$arm.jsonl" 2>"$M/out/g1b-$arm.stderr") || true
	done
	check init "$M/out/g1b-off.jsonl" "$P1_off" "$M/out/g1b-steer.jsonl" "$P1_steer" "$model" "$PIN"
	tarball "$P1_off" g1b-off
	tarball "$P1_steer" g1b-steer
	for arm in off steer; do
		newarm
		check addhook "$A/cc/settings.json" "{ env | sort; go env; git config --list --show-origin; } > '$A/envdump.txt' 2>&1"
		session "$A" "$arm" 1 "g1c-$arm"
		put "P_$arm" "$A"
		put "PWS_$arm" "$WS"
		put "PSID_$arm" "$(cat "$M/out/g1c-$arm.session")"
		cp "$A/envdump.txt" "$M/out/g1a-$arm.env" 2>/dev/null || die "G1a: the $arm probe's hook wrote no dump"
	done
	check projection "$NPROMPTS" "$(check transcript "$P_off/cc" "$PSID_off")" "$(check transcript "$P_steer/cc" "$PSID_steer")"
	check envdiff "$M/out/g1a-off.env" "$P_off" "$PWS_off" "$PSID_off" "$M/out/g1a-steer.env" "$P_steer" "$PWS_steer" "$PSID_steer"
	text=$(check control "$P_steer/store" "$PSID_steer" "$P_off/store" "$PSID_off") || { printf '%s\n' "$text" >&2; die "G1c: the positive control failed"; }
	tr=$(check transcript "$P_steer/cc" "$PSID_steer")
	S2_CONTROL=0
	if [ -n "$tr" ] && check holds "$tr" "$text"; then
		S2_CONTROL=1
		say "G1c control seen; the steer transcript holds the delivered note (S2's additionalContext control)"
	else
		say "G1c control seen; the steer transcript does not hold the delivered note: S2 has no additionalContext control"
	fi
	check claudemd "$PWS_steer" "$P_steer/cc" "$policy"
	plant=$(mktemp -d "${TMPDIR:-/tmp}/measure1-plant.XXXXXX")
	mkdir "$plant/ws" "$plant/none"
	echo planted >"$plant/CLAUDE.md"
	if check claudemd "$plant/ws" "$plant/none" "$policy" >/dev/null; then die "G1d: the walk missed a planted CLAUDE.md"; fi
	rm "$plant/CLAUDE.md"
	decoy=$(mktemp -d "${TMPDIR:-/tmp}/measure1-decoy.XXXXXX")
	echo decoy >"$decoy/CLAUDE.md"
	if check claudemd "$plant/ws" "$decoy" "$policy" >/dev/null; then die "G1d: the walk missed the decoy config's CLAUDE.md"; fi
	rm -rf "$plant" "$decoy"
	say "G1d controls: the walk finds a planted ancestor file and a decoy config's"
	check repoclaude "$PWS_steer"
	tarball "$P_off" g1c-off
	tarball "$P_steer" g1c-steer
}

# g2 builds the oracle apart from M: FAIL_TO_PASS red on base
# with the fix's added tests, everything green on the reference.
g2() {
	O=$(mktemp -d "${TMPDIR:-/tmp}/measure1-oracle.XXXXXX")
	mkdir "$O/base" "$O/ref"
	git -C "$task/repo" archive "$base" | tar -x -C "$O/base"
	git -C "$task/repo" archive "$fix" | tar -x -C "$O/ref"
	git -C "$task/repo" diff --name-only --diff-filter=AM "$base" "$fix" -- '*_test.go' >"$O/added"
	[ -s "$O/added" ] || die "G2: the fix adds no test file"
	(cd "$O/ref" && GOMODCACHE="$M/modcache" GOFLAGS=-mod=mod "$M/bin/go" mod download) ||
		die "G2: go mod download failed"
	chmod -R a-w "$M/modcache"
	cp -R "$O/base" "$O/red"
	while IFS= read -r f; do mkdir -p "$O/red/$(dirname "$f")" && cp "$O/ref/$f" "$O/red/$f"; done <"$O/added"
	oracle "$O/red" "$O/red.json"
	check oracle "$O/red.json" "$task/FAIL_TO_PASS" "$task/PASS_TO_PASS" red
	oracle "$O/ref" "$O/ref.json"
	check oracle "$O/ref.json" "$task/FAIL_TO_PASS" "$task/PASS_TO_PASS" green
	cp "$O/red.json" "$O/ref.json" "$M/out/"
}

# oracle runs go test -json in DIR: FAIL_TO_PASS by name, PASS_TO_PASS by
# package, under the arms' module rules.
oracle() {
	local dir=$1 out=$2 names pkgs
	names=$(awk '{print $2}' "$task/FAIL_TO_PASS" | paste -sd'|' -)
	pkgs=$(awk '{print $1}' "$task/FAIL_TO_PASS" | sort -u | tr '\n' ' ')
	(cd "$dir" && GOMODCACHE="$M/modcache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local GOENV=off \
		GOCACHE="$M/oracle-gocache" "$M/bin/go" test -count=1 -json -run "^($names)\$" $pkgs >"$out" 2>"$out.stderr") || true
	[ -s "$task/PASS_TO_PASS" ] || return 0
	(cd "$dir" && GOMODCACHE="$M/modcache" GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local GOENV=off \
		GOCACHE="$M/oracle-gocache" "$M/bin/go" test -count=1 -json $(tr '\n' ' ' <"$task/PASS_TO_PASS") >>"$out" 2>>"$out.stderr") || true
}

run() {
	M=$1
	shift
	[ -d "$M/tpl" ] || die "run: $M is not a prepared directory"
	task="" model=claude-haiku-4-5-20251001 order=seed:1 reps=1 limit=900 allowed="Bash,Edit,Write,Read,Glob,Grep"
	while [ $# -gt 0 ]; do
		case $1 in
		--task) task=$2 ;;
		--model) model=$2 ;;
		--order) order=$2 ;;
		--reps) reps=$2 ;;
		--limit) limit=$2 ;;
		--allowed) allowed=$2 ;;
		*) die "run: unknown argument $1" ;;
		esac
		shift 2
	done
	[ -n "$task" ] || die "run: --task DIR is required"
	task=$(cd "$task" && pwd)
	base=$(cat "$task/base")
	fix=$(cat "$task/fix")
	ARMS=()
	trap cleanup EXIT
	prompts
	g0
	g2
	g1
	local n=0 arm label
	for arm in $(check order "${order#seed:}" "$reps"); do
		n=$((n + 1))
		label="arm-$n-$arm"
		newarm
		echo "$A" >"$M/out/$label.root"
		session "$A" "$arm" "$NPROMPTS" "$label"
		say "$label session $(cat "$M/out/$label.session"): $(tr '\n' ';' <"$M/out/$label.turns")"
		tarball "$A" "$label"
		[ "$(shasum -a 256 "$M/bin/rashomon" | awk '{print $1}')" = "$BIN_SHA" ] || die "G0: M/bin/rashomon changed during $label"
	done
	report
	[ "$("$claude" --version | awk '{print $1}')" = "$PIN" ] || die "G0: claude's version moved during the run"
}

# report runs G4, G5 and G6 over the arms' tars.
report() {
	local t label arm x scratch both=1 out others
	for t in "$M"/out/arm-*.tar; do
		label=$(basename "$t" .tar)
		arm=${label##*-}
		x="$M/out/$label"
		mkdir -p "$x"
		tar -C "$x" -xf "$t"
		scratch=$(mktemp -d "${TMPDIR:-/tmp}/measure1-g4.XXXXXX")
		cp -R "$x"/ws-*/. "$scratch/"
		while IFS= read -r f; do mkdir -p "$scratch/$(dirname "$f")" && cp "$O/ref/$f" "$scratch/$f"; done <"$O/added"
		oracle "$scratch" "$M/out/$label.oracle.json"
		rm -rf "$scratch"
		check oracle "$M/out/$label.oracle.json" "$task/FAIL_TO_PASS" "$task/PASS_TO_PASS" score | sed "s/^measure1: /measure1: G4 $label: /"
		check oracle "$M/out/$label.oracle.json" "$task/FAIL_TO_PASS" "$task/PASS_TO_PASS" green >/dev/null || both=0
		put "SID_$arm" "$(cat "$M/out/$label.session")"
		put "SIDE_$arm" "$x"
		put "ROOT_$arm" "$(cat "$M/out/$label.root")"
	done
	out=$(check sensors off "$SIDE_off/store" "$SID_off" "$(check transcript "$SIDE_off/cc" "$SID_off")" "$S2_CONTROL" "$M" "$O" "$ROOT_steer") ||
		{ printf '%s\n' "$out" >&2; die "G5: a sensor failed on the off arm"; }
	printf '%s\n' "$out"
	out=$(check sensors steer "$SIDE_steer/store" "$SID_steer" "$(check transcript "$SIDE_steer/cc" "$SID_steer")" "$S2_CONTROL" "$M" "$O" "$ROOT_off") ||
		{ printf '%s\n' "$out" >&2; die "G5: a sensor failed on the steer arm"; }
	printf '%s\n' "$out"
	set -- --session "$SID_off" --arm off --home "$SIDE_off/store" --config "$SIDE_off/cc" \
		--session "$SID_steer" --arm steer --home "$SIDE_steer/store" --config "$SIDE_steer/cc"
	"$M/bin/rashomon" measure pair "$@" | tee "$M/out/pair.txt"
	"$M/bin/rashomon" measure pair "$@" --json >"$M/out/pair.json"
	if [ "$both" = 1 ]; then say "undiscriminated: both arms pass every FAIL_TO_PASS test"; fi
	say "results in $M/out"
}

case ${1:-} in
prepare)
	[ $# -ge 2 ] || die "usage: measure1.sh prepare BIN [DIR]"
	prepare "$2" "${3:-}"
	;;
run)
	[ $# -ge 2 ] || die "usage: measure1.sh run M --task DIR [...]"
	shift
	run "$@"
	;;
task)
	[ $# -eq 4 ] && [ "$3" = --task ] || die "usage: measure1.sh task M --task DIR"
	M=$2
	[ -x "$M/bin/go" ] || die "task: $M is not a prepared directory"
	task=$(cd "$4" && pwd)
	base=$(cat "$task/base")
	fix=$(cat "$task/fix")
	g2
	say "G2 the task holds: FAIL_TO_PASS red on base, the reference green"
	;;
*) die "usage: measure1.sh prepare BIN [DIR] | run M --task DIR [--model ID] [--order seed:N] [--reps K] [--limit S] [--allowed TOOLS] | task M --task DIR" ;;
esac
