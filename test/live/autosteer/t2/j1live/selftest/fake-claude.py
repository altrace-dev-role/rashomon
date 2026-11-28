#!/usr/bin/env python3
"""fake-claude.py: a scripted stand-in for the 2.1.322 TUI, for drive.py's offline self-test (selftest.sh).

Draws the screens drive.py must pass, with 2.1.322's strings: the theme picker, the security notes, the terminal setup
offer, the folder trust dialog (no indexes, "No, exit" first and focused, as the binary builds it), then two plan
dialogs whose "No, keep planning" option is an input. Reads keys from the pane's tty in raw mode. Writes transcript
records in 2.1.322's shapes to $CLAUDE_CONFIG_DIR/projects/selftest/<session id>.jsonl: ExitPlanMode tool_use, its
tool_result (an error on the rejection), PreToolUse hook_success for both calls, a PostToolUse:ExitPlanMode
hook_additional_context for the accepted call only, then an Edit tool_use and result and an end_turn reply.
FAKE_PRE_LINE=1 adds a PreToolUse:ExitPlanMode hook_additional_context on both calls (read.py must flag it).
FAKE_UNKNOWN=1 shows, after the trust dialog, a numbered dialog drive.py has no rule for; any answer to it exits 16,
and only Escape (drive.py's end sequence after its abort) leads on to the idle prompt.
Any wrong choice exits non-zero with the reason in $FAKE_OUT/fake.log; /exit at the prompt exits 0.
No network, no model, no meter.
"""
import json, os, sys, termios, time, tty

PTR = "❯"
args = sys.argv[1:]
OUT = os.environ["FAKE_OUT"]
LOGF = open(os.path.join(OUT, "fake.log"), "a")


def log(m):
    LOGF.write(m + "\n")
    LOGF.flush()


with open(os.path.join(OUT, "fake-argv.json"), "w") as f:
    json.dump(args, f)
SID = args[args.index("--session-id") + 1]
TP = os.path.join(os.environ["CLAUDE_CONFIG_DIR"], "projects", "selftest", SID + ".jsonl")
os.makedirs(os.path.dirname(TP), exist_ok=True)
FD = 0
OLD = termios.tcgetattr(FD)


def die(code, why):
    termios.tcsetattr(FD, termios.TCSADRAIN, OLD)
    log("FAIL %d %s" % (code, why))
    sys.exit(code)


def rec(d):
    d.setdefault("sessionId", SID)
    with open(TP, "a") as f:
        f.write(json.dumps(d) + "\n")


if args[0].startswith("-") or args[args.index("--permission-mode") + 1] != "plan":
    die(10, "argv: the prompt is not first or the mode is not plan: %r" % args)
if os.environ.get("DISABLE_AUTOUPDATER") != "1":
    die(11, "DISABLE_AUTOUPDATER is not 1")
tty.setraw(FD)


def draw(lines):
    sys.stdout.write("\x1b[2J\x1b[H" + "\r\n".join(lines) + "\r\n")
    sys.stdout.flush()


def keys():
    b = os.read(FD, 256).decode(errors="replace")
    out, i = [], 0
    while i < len(b):
        if b.startswith("\x1b[A", i) or b.startswith("\x1bOA", i):
            out.append("UP"); i += 3
        elif b.startswith("\x1b[B", i) or b.startswith("\x1bOB", i):
            out.append("DOWN"); i += 3
        elif b[i] == "\x1b":
            out.append("ESC"); i += 1
        elif b[i] in "\r\n":
            out.append("ENTER"); i += 1
        elif b[i] == "\x7f":
            out.append("BS"); i += 1
        else:
            out.append(b[i]); i += 1
    return out


def select(title, options, indexes=True, input_idx=None):
    focus, typed = 0, ""
    while True:
        lines = list(title) + [""]
        for k, o in enumerate(options):
            label = o
            if k == input_idx and k == focus:
                label = "%s > %s" % (o, typed or "Tell Claude what to change")
            lines.append(" %s %s%s" % (PTR if k == focus else " ", "%d. " % (k + 1) if indexes else "", label))
        draw(lines)
        for k in keys():
            if k == "UP":
                focus = max(0, focus - 1)
            elif k == "DOWN":
                focus = min(len(options) - 1, focus + 1)
            elif k == "ENTER":
                return focus, typed
            elif k == "ESC":
                return -1, typed
            elif k == "BS":
                typed = typed[:-1]
            elif len(k) == 1 and focus == input_idx:
                typed += k


def idle_prompt():
    typed = ""
    while True:
        draw(["> " + typed, "", "? for shortcuts"])
        for k in keys():
            if k == "ENTER":
                if typed.strip() == "/exit":
                    termios.tcsetattr(FD, termios.TCSADRAIN, OLD)
                    log("exit via /exit")
                    sys.exit(0)
                typed = ""
            elif k == "ESC":
                typed = ""
            elif k == "BS":
                typed = typed[:-1]
            elif len(k) == 1:
                typed += k


def working(n):
    for _ in range(n):
        draw(["* Planning... (esc to interrupt)"])
        time.sleep(1)


def assistant(mid, content, stop):
    rec({"type": "assistant", "message": {"id": mid, "model": "claude-sonnet-5", "role": "assistant", "stop_reason": stop,
         "content": content, "usage": {"input_tokens": 1000, "cache_creation_input_tokens": 2000,
                                       "cache_read_input_tokens": 10000, "output_tokens": 300}}})


def attach(a):
    rec({"type": "attachment", "attachment": a})


def result(tid, err, text):
    rec({"type": "user", "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": tid,
                                                                    "is_error": err, "content": text}]}})


i, _ = select(["Let's get started.", "Choose the text style that looks best with your terminal"], ["Dark mode", "Light mode"])
log("theme %d" % i)
draw(["Security notes:", "", "Claude can make mistakes.", "", "Press Enter to continue"])
while "ENTER" not in keys():
    pass
log("notes")
i, _ = select(["Use Claude Code's terminal setup?", "For the optimal coding experience, enable the recommended settings"],
              ["Yes, use recommended settings", "No, maybe later with /terminal-setup"])
if i != 1:
    die(12, "terminal setup: chose %d" % i)
log("termsetup no")
i, _ = select(["Quick safety check: Is this a project you created or one you trust?"],
              ["No, exit", "Yes, I trust this folder"], indexes=False)
if i != 1:
    die(13, "trust: chose %d" % i)
log("trust yes")
if os.environ.get("FAKE_UNKNOWN") == "1":
    i, _ = select(["A screen this driver has never seen"], ["Option A", "Option B"])
    if i != -1:
        die(16, "unknown: chose %d (the driver must not answer a dialog it has no rule for)" % i)
    log("unknown escaped")
    idle_prompt()
rec({"type": "user", "message": {"role": "user", "content": args[0]}})
PLAN = ["Ready to code?", "", "Here is Claude's plan:", "  add a doc comment above Apply in internal/install/install.go", "",
        "Claude has written up a plan and is ready to execute. Would you like to proceed?"]
OPTS = ["Yes, auto-accept edits", "Yes, manually approve edits", "No, keep planning"]
for n, eid in enumerate(("toolu_fake_e1", "toolu_fake_e2"), 1):
    working(3)
    assistant("msg_plan%d" % n, [{"type": "tool_use", "id": eid, "name": "ExitPlanMode", "input": {"plan": "p"}}], "tool_use")
    attach({"type": "hook_success", "hookName": "PreToolUse:ExitPlanMode", "toolUseID": eid, "hookEvent": "PreToolUse",
            "stdout": "", "exitCode": 0})
    if os.environ.get("FAKE_PRE_LINE") == "1":
        attach({"type": "hook_additional_context", "hookName": "PreToolUse:ExitPlanMode", "toolUseID": eid,
                "hookEvent": "PreToolUse", "content": ["fake pre plan line"]})
    i, typed = select(PLAN, OPTS, input_idx=2)
    if n == 1:
        if i != 2 or not typed.startswith("Revise the plan"):
            die(14, "plan 1: chose %d typed %r" % (i, typed))
        log("plan 1 rejected: %s" % typed)
        result(eid, True, "The user doesn't want to proceed with this tool use. Feedback: " + typed)
    else:
        if i != 1:
            die(15, "plan 2: chose %d" % i)
        log("plan 2 accepted")
        result(eid, False, "User has approved your plan. You can now start coding.")
        attach({"type": "hook_success", "hookName": "PostToolUse:ExitPlanMode", "toolUseID": eid,
                "hookEvent": "PostToolUse", "stdout": "{}", "exitCode": 0})
        attach({"type": "hook_additional_context", "hookName": "PostToolUse:ExitPlanMode", "toolUseID": eid,
                "hookEvent": "PostToolUse", "content": ["fake post plan line"]})
working(2)
assistant("msg_edit", [{"type": "tool_use", "id": "toolu_fake_ed1", "name": "Edit", "input": {}}], "tool_use")
result("toolu_fake_ed1", False, "edited")
assistant("msg_end", [{"type": "text", "text": "Done."}], "end_turn")
idle_prompt()
