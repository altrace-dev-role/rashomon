#!/usr/bin/env python3
"""drive.py: J1-LIVE's stand-in for the CLI (claude-tmux.sh execs it; run.sh calls that as RK_CLAUDE_BIN).

Pre-registration r3 section 8 (the plan line's channel is POST only, the director 2026-11-11 16:40:01 PST).
run.sh starts `CLI -p PROMPT --model M --session-id SID --allowedTools LIST --permission-mode X --max-turns N
--max-budget-usd U --output-format json` in the run's tree with the run's environment. This program starts the real
CLI (J1_REAL_CLAUDE) INTERACTIVELY instead, in a tmux pane on its own socket, with the same model, session id,
allowed tools, environment and working directory, in --permission-mode plan, the prompt as the FIRST argument
(--allowedTools takes a variadic list and would swallow a prompt placed after it), and DISABLE_AUTOUPDATER=1 (r3
QA-ADD 2). -p, --max-turns, --max-budget-usd and --output-format have no interactive meaning and are dropped; the
cost watchdog stands in for the budget. Then it drives the pane, matching the 2.1.322 strings:
  theme       "Choose the text style": Enter keeps the focused default
  notes       "Security notes:": Enter
  termsetup   "Use Claude Code's terminal setup?": its "No" line (Yes would rewrite the terminal's own settings)
  trust       "Yes, I trust this folder": no digits in 2.1.322 (hideIndexes, "No, exit" first and focused), so
              the pointer is moved to the Yes line and checked before Enter
  lsp         "Would you like to install this LSP plugin?": "No, not now"
  plugin      "Would you like to install it?": its first "No" line
  plan 1      REJECT: the pointer to "No, keep planning" (an input option), the feedback typed, checked, Enter
  plan 2      ACCEPT: the pointer to "Yes, manually approve edits" (else the first "Yes" line), checked, Enter
  permission  "Do you want to" after the acceptance: Escape (the edit's tool_use is already recorded); before: abort
  unknown     a pointer-marked numbered option list unchanged for 20 s: abort
Every key is sent after a fresh capture shows the expected screen, and Enter only with the pointer on the target
line (the 10-31 pitfall: an offer can close on its own and a key then lands in the prompt box as a model turn).
A plan dialog is answered only once the transcript holds as many ExitPlanMode calls as dialogs seen (10 s grace).
Stop: after the accepted exit's tool_result, the first Edit, Write or MultiEdit tool_result after it (plus 5 s), an
end_turn reply, or 120 s; then Escape, /exit, and the tmux server is killed after at most 20 s.
Abort: an unknown dialog; a third plan dialog; a permission prompt before the acceptance; idle 90 s with no dialog and
the model not working; the transcript estimate over 0.40 USD (cap 0.50; main-model requests only, Sonnet 5 list
prices, the same arithmetic as read.py's usd()); 900 s wall clock.
Writes RK_RUN_DIR/j1/: argv.json, env-names.txt, launch.sh, pane-NNN.txt (each distinct capture), steps.tsv,
drive.log, claude-exit (written by launch.sh). env.sh (mode 600, the full environment) is removed at the end.
Prints one JSON result line (run.sh saves it as result.json). Exit 0 when the plan sequence completed, 1 on an
abort, 2 on a usage error.
"""
import glob, json, os, re, shlex, subprocess, sys, time

FLAGS = {"-p": "prompt", "--model": "model", "--session-id": "sid", "--allowedTools": "tools",
         "--permission-mode": "mode", "--max-turns": "max_turns", "--max-budget-usd": "budget",
         "--output-format": "fmt"}
FEEDBACK = "Revise the plan: also name the test that would cover the comment, then present it again."
ABORT_USD, CAP_USD = 0.40, 0.50
WALL_S, POLL_S, UNKNOWN_S, IDLE_S, AFTER_ACCEPT_S = 900, 2.0, 20, 90, 120
PTR = "\u276f\u203a\u25b8"  # the select pointer glyphs; ">" is left out: it is the prompt box's own marker
BORDER = " \t\u2502\u2503|"  # a dialog's left border, stripped before a line is read
EDITS = ("Edit", "Write", "MultiEdit")


def result(subtype, is_error, extra):
    d = {"type": "result", "subtype": subtype, "is_error": is_error}
    d.update(extra)
    print(json.dumps(d, sort_keys=True))


def parse(argv):
    got, i = {}, 0
    while i < len(argv):
        k = argv[i]
        if k not in FLAGS or i + 1 >= len(argv):
            return None, "unknown or valueless argument %r" % k
        got[FLAGS[k]] = argv[i + 1]
        i += 2
    missing = [k for k in ("prompt", "model", "sid", "tools") if k not in got]
    return (None, "missing " + ",".join(missing)) if missing else (got, None)


A, err = parse(sys.argv[1:])
if err or A is None:
    result("j1live_usage", True, {"error": err})
    sys.exit(2)
REAL = os.environ.get("J1_REAL_CLAUDE", "")
RUN = os.environ.get("RK_RUN_DIR", "")
CFG = os.environ.get("CLAUDE_CONFIG_DIR", "")
TMUX = os.environ.get("J1_TMUX", "/opt/homebrew/bin/tmux")
for name, val in (("J1_REAL_CLAUDE", REAL), ("RK_RUN_DIR", RUN), ("CLAUDE_CONFIG_DIR", CFG)):
    if not val:
        result("j1live_usage", True, {"error": name + " unset"})
        sys.exit(2)
J = os.path.join(RUN, "j1")
os.makedirs(J)
SID = A["sid"]
SOCK = "j1-" + SID[:8]
CWD = os.getcwd()
T0 = time.time()
LOG = open(os.path.join(J, "drive.log"), "a")
STEPS = open(os.path.join(J, "steps.tsv"), "a")
STEPS.write("utc\telapsed_s\tkind\taction\tdetail\n")


def utc():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def log(msg):
    LOG.write("%s %6.1f %s\n" % (utc(), time.time() - T0, msg))
    LOG.flush()


def step(kind, action, detail=""):
    STEPS.write("%s\t%.1f\t%s\t%s\t%s\n" % (utc(), time.time() - T0, kind, action, detail.replace("\t", " ")))
    STEPS.flush()
    log("%s: %s %s" % (kind, action, detail))


# The pane's environment: run.sh's, less the terminal's own variables (tmux sets TERM; TMUX* would nest).
SKIP = {"TERM", "TMUX", "TMUX_PANE", "COLUMNS", "LINES", "PWD", "OLDPWD", "SHLVL", "_"}
names = sorted(k for k in os.environ if k not in SKIP and re.match(r"^[A-Za-z_][A-Za-z0-9_]*$", k))
envsh = os.path.join(J, "env.sh")
fd = os.open(envsh, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w") as f:
    for k in names:
        f.write("export %s=%s\n" % (k, shlex.quote(os.environ[k])))
with open(os.path.join(J, "env-names.txt"), "w") as f:
    f.write("\n".join(names) + "\nDISABLE_AUTOUPDATER (set by launch.sh)\n")
with open(os.path.join(J, "argv.json"), "w") as f:
    json.dump({"received": {k: v for k, v in A.items() if k != "prompt"}, "prompt_chars": len(A["prompt"]),
               "launched_mode": "plan", "dropped": [k for k in ("mode", "max_turns", "budget", "fmt") if k in A]},
              f, indent=1, sort_keys=True)
EXITF = os.path.join(J, "claude-exit")
q = shlex.quote
cmd = " ".join([q(REAL), q(A["prompt"]), "--model", q(A["model"]), "--session-id", q(SID),
                "--allowedTools", q(A["tools"]), "--permission-mode", "plan"])
launch = os.path.join(J, "launch.sh")
with open(launch, "w") as f:
    f.write("#!/bin/sh\n# J1-LIVE pane command, written by drive.py. TERM stays tmux's; the rest is run.sh's.\n"
            "t=$TERM\n. %s\nexport TERM=\"$t\" DISABLE_AUTOUPDATER=1\n"
            "cd %s || { echo 97 > %s; exit 97; }\n%s\necho $? > %s\nsleep 30\n"
            % (q(envsh), q(CWD), q(EXITF), cmd, q(EXITF)))

TENV = {k: v for k, v in os.environ.items() if not k.startswith("TMUX")}
if TENV.get("TERM", "dumb") == "dumb":
    TENV["TERM"] = "xterm-256color"


def tm(*args):
    return subprocess.run([TMUX, "-L", SOCK, "-f", "/dev/null"] + list(args), capture_output=True, text=True, env=TENV)


def capture():
    r = tm("capture-pane", "-p", "-t", "j1")
    return r.stdout if r.returncode == 0 else None


def key(*names):
    tm("send-keys", "-t", "j1", *names)


def literal(text):
    tm("send-keys", "-t", "j1", "-l", text)


# ---- transcript (the same record reading as read.py)
def records():
    for p in glob.glob(os.path.join(CFG, "projects", "*", SID + ".jsonl")):
        out = []
        for line in open(p, errors="replace"):
            try:
                out.append(json.loads(line))
            except ValueError:
                out.append({})
        return out
    return []


def blocks(d):
    m = d.get("message")
    c = m.get("content") if isinstance(m, dict) else None
    return [b for b in c if isinstance(b, dict)] if isinstance(c, list) else []


def usd(recs):
    """Kept equal to read.py's usd(): Sonnet 5 list prices per MTok, usage deduplicated by message id."""
    per, models = {}, set()
    for d in recs:
        if d.get("type") != "assistant":
            continue
        m = d.get("message") or {}
        u = m.get("usage") or {}
        models.add(m.get("model"))
        mid = m.get("id") or id(d)
        p = per.setdefault(mid, {})
        for k in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens"):
            p[k] = max(p.get(k, 0), int(u.get(k) or 0))
        cc = u.get("cache_creation") or {}
        p["h1"] = max(p.get("h1", 0), int(cc.get("ephemeral_1h_input_tokens") or 0))
    total = 0.0
    for p in per.values():
        cw = p["cache_creation_input_tokens"]
        h1 = min(p["h1"], cw)
        total += (p["input_tokens"] * 2.0 + (cw - h1) * 2.5 + h1 * 4.0 + p["cache_read_input_tokens"] * 0.2
                  + p["output_tokens"] * 10.0) / 1e6
    return total, len(per), sorted(str(x) for x in models)


def exits(recs):
    """ExitPlanMode tool_use ids in order, and {id: (record number, is_error)} for their tool_results."""
    ids, res = [], {}
    for i, d in enumerate(recs, 1):
        for b in blocks(d):
            if b.get("type") == "tool_use" and b.get("name") == "ExitPlanMode" and b.get("id") not in ids:
                ids.append(b.get("id"))
            if b.get("type") == "tool_result" and b.get("tool_use_id") in ids and b.get("tool_use_id") not in res:
                res[b.get("tool_use_id")] = (i, bool(b.get("is_error")))
    return ids, res


def after_accept(recs, acc_res_rec):
    """After the accepted exit's tool_result: the first edit tool_use, whether its tool_result exists, and whether an
    end_turn reply came."""
    edit_id, edit_done, end_turn = None, False, False
    for i, d in enumerate(recs, 1):
        if i <= acc_res_rec:
            continue
        for b in blocks(d):
            if b.get("type") == "tool_use" and b.get("name") in EDITS and edit_id is None:
                edit_id = b.get("id")
            if b.get("type") == "tool_result" and edit_id and b.get("tool_use_id") == edit_id:
                edit_done = True
        m = d.get("message")
        if d.get("type") == "assistant" and isinstance(m, dict) and m.get("stop_reason") == "end_turn":
            end_turn = True
    return edit_id, edit_done, end_turn


# ---- screens
def classify(s):
    if "Choose the text style" in s:
        return "theme"
    if "Use Claude Code's terminal setup?" in s:
        return "termsetup"
    if "Security notes:" in s:
        return "notes"
    if "Yes, I trust this folder" in s:
        return "trust"
    if "Would you like to install this LSP plugin?" in s:
        return "lsp"
    if "Would you like to install it?" in s:
        return "plugin"
    if "Would you like to proceed?" in s or "No, keep planning" in s:
        return "plan"
    if "Do you want to" in s and re.search(r"^[%s]*[%s]?\s*\d+\.\s" % (BORDER, PTR), s, re.M):
        return "permission"
    if re.search(r"^[%s]*[%s]\s*\d+\.\s" % (BORDER, PTR), s, re.M):
        return "unknown"
    return "none"


def pointer_line(lines, near):
    """The pointer-marked line nearest line index `near` (a dialog's own pointer, not another region's)."""
    cand = [i for i, l in enumerate(lines) if l.lstrip(BORDER)[:1] and l.lstrip(BORDER)[:1] in PTR]
    return min(cand, key=lambda i: abs(i - near)) if cand else None


def move_to(kind, targets):
    """Move the pointer to the first line holding any of `targets`; True when a fresh capture shows it there."""
    for _ in range(10):
        s = capture()
        if s is None or classify(s) != kind:
            step(kind, "abandoned", "screen changed while moving")
            return False
        lines = s.splitlines()
        tgt = next((i for i, l in enumerate(lines) if any(t in l for t in targets)), None)
        if tgt is None:
            step(kind, "abandoned", "no line holds %s" % targets)
            return False
        p = pointer_line(lines, tgt)
        if p is None:
            step(kind, "abandoned", "no pointer on screen")
            return False
        if any(t in lines[p] for t in targets):
            return True
        key("Down" if tgt > p else "Up")
        time.sleep(0.5)
    step(kind, "abandoned", "pointer did not reach %s in 10 moves" % targets)
    return False


def first_label(s, word):
    """The first option label starting with the whole word `word` (No, Yes; never Note or Yesterday)."""
    for l in s.splitlines():
        t = re.sub(r"^[%s]*[%s]?\s*(?:\d+\.\s+)?" % (BORDER, PTR), "", l).strip()
        if re.match(word + r"\b", t):
            return t
    return None


def choose(kind, targets):
    if move_to(kind, targets):
        key("Enter")
        step(kind, "enter", "on %s" % targets[0])
        return True
    return False


# ---- run
started = tm("new-session", "-d", "-s", "j1", "-x", "200", "-y", "50", "sh " + q(launch))
if started.returncode != 0:
    result("j1live_abort", True, {"session_id": SID, "error": "tmux new-session: " + started.stderr.strip()})
    sys.exit(1)
step("start", "tmux", "socket %s; %s" % (SOCK, TMUX))
paned, last, last_change, handled = 0, None, time.time(), None
plan_seen, accepted_id, accept_t, acc_res = 0, None, None, None
acc_res_t, finish_at, why, aborted = None, None, None, None
last_tr_len, last_tr_t = 0, time.time()
while True:
    now = time.time()
    if os.path.exists(EXITF):
        why = why or "claude exited (%s)" % open(EXITF).read().strip()
        break
    if now - T0 > WALL_S:
        aborted = "wall clock %d s" % WALL_S
        break
    recs = records()
    if len(recs) != last_tr_len:
        last_tr_len, last_tr_t = len(recs), now
    cost, nmsg, models = usd(recs)
    if cost > ABORT_USD:
        aborted = "transcript estimate %.4f USD over %.2f" % (cost, ABORT_USD)
        break
    s = capture()
    if s is None:
        why = "pane gone"
        break
    if s != last:
        paned += 1
        if paned <= 400:
            with open(os.path.join(J, "pane-%03d.txt" % paned), "w") as f:
                f.write("# %s elapsed %.1f\n%s" % (utc(), now - T0, s))
        last, last_change = s, now
    kind = classify(s)
    if handled and kind != handled:
        handled = None
    ids, res = exits(recs)
    if accepted_id:
        if acc_res is None and accepted_id in res:
            acc_res, acc_res_t = res[accepted_id], now
            step("stop", "seen", "accepted exit's tool_result at record %d is_error=%s" % acc_res)
        if acc_res and finish_at is None:
            edit_id, edit_done, end_turn = after_accept(recs, acc_res[0])
            if edit_done:
                finish_at, why = now + 5, "first edit's tool_result seen (%s)" % edit_id
            elif end_turn:
                finish_at, why = now + 3, "end_turn reply after the acceptance"
            elif acc_res_t is not None and now - acc_res_t > AFTER_ACCEPT_S:
                finish_at, why = now, "no edit within %d s of the acceptance" % AFTER_ACCEPT_S
        if acc_res is None and accept_t is not None and now - accept_t > 60:
            finish_at, why = now, "no tool_result for the accepted exit within 60 s"
        if finish_at and now >= finish_at:
            break
    if handled is None and kind != "none":
        if kind == "theme":
            key("Enter"); step(kind, "enter", "focused default"); handled = kind
        elif kind == "notes":
            key("Enter"); step(kind, "enter"); handled = kind
        elif kind == "termsetup":
            lab = first_label(s, "No")
            if lab and choose(kind, [lab]):
                handled = kind
        elif kind == "trust":
            if choose(kind, ["Yes, I trust this folder"]):
                handled = kind
        elif kind == "lsp":
            if choose(kind, ["No, not now"]):
                handled = kind
        elif kind == "plugin":
            lab = first_label(s, "No")
            if lab and choose(kind, [lab]):
                handled = kind
        elif kind == "plan":
            want = plan_seen + 1
            if len(ids) < want and now - last_change < 10:
                pass  # the transcript lags the dialog; wait up to 10 s
            elif want == 1:
                step("plan", "dialog 1", "transcript ExitPlanMode calls %d" % len(ids))
                if move_to("plan", ["No, keep planning", "Tell Claude what to change"]):
                    literal(FEEDBACK)
                    time.sleep(0.8)
                    s2 = capture() or ""
                    if FEEDBACK[:40] in s2.replace("\n", " "):
                        key("Enter")
                        step("plan", "reject", "feedback typed and Enter")
                        plan_seen, handled = 1, "plan"
                    else:
                        aborted = "feedback text not visible after typing"
                        break
            elif want == 2:
                step("plan", "dialog 2", "transcript ExitPlanMode calls %d" % len(ids))
                lab = "Yes, manually approve edits" if "Yes, manually approve edits" in s else first_label(s, "Yes")
                if lab and choose("plan", [lab]):
                    plan_seen, handled = 2, "plan"
                    accepted_id, accept_t = (ids[-1] if ids else None), now
                    step("plan", "accept", "%s; exit id %s" % (lab, accepted_id))
                    if accepted_id is None:
                        aborted = "accepted a plan the transcript does not show"
                        break
            else:
                aborted = "a third plan dialog"
                break
        elif kind == "permission":
            if accepted_id:
                key("Escape"); step(kind, "escape", "after the acceptance"); handled = kind
                finish_at, why = now + 3, why or "permission prompt after the acceptance (escaped)"
            else:
                aborted = "permission prompt before the acceptance"
                break
        elif kind == "unknown" and now - last_change > UNKNOWN_S:
            aborted = "unknown dialog unchanged %d s" % UNKNOWN_S
            break
    working = "esc to interrupt" in s
    if kind == "none" and not working and not accepted_id and now - last_change > IDLE_S and now - last_tr_t > IDLE_S:
        aborted = "idle %d s with no dialog and the model not working" % IDLE_S
        break
    time.sleep(POLL_S)

# ---- end the session
if not os.path.exists(EXITF):
    key("Escape"); time.sleep(1.5)
    literal("/exit"); time.sleep(0.3); key("Enter")
    step("end", "exit", "Escape, /exit, Enter")
    for _ in range(20):
        if os.path.exists(EXITF):
            break
        time.sleep(1)
s = capture()
if s is not None:
    with open(os.path.join(J, "pane-final.txt"), "w") as f:
        f.write("# %s elapsed %.1f\n%s" % (utc(), time.time() - T0, s))
tm("kill-server")
step("end", "kill-server", "claude-exit %s" % (open(EXITF).read().strip() if os.path.exists(EXITF) else "absent"))
os.remove(envsh)
recs = records()
cost, nmsg, models = usd(recs)
ids, res = exits(recs)
summary = {"session_id": SID, "total_cost_usd": round(cost, 6), "cost_basis": "transcript estimate, main model only",
           "cap_usd": CAP_USD, "assistant_messages": nmsg, "models": models, "exit_plan_calls": ids,
           "plan_dialogs_answered": plan_seen, "accepted_exit_id": accepted_id, "stop": why, "abort": aborted,
           "elapsed_s": round(time.time() - T0, 1), "panes": paned}
step("end", "result", json.dumps(summary, sort_keys=True))
complete = aborted is None and plan_seen == 2 and acc_res is not None
result("j1live" if complete else "j1live_abort", not complete, summary)
sys.exit(0 if complete else 1)
