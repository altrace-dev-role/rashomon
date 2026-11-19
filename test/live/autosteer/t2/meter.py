"""meter.py: T2's one meter reader and gate (the re-cut; the director 14:13 PST 11-14 items 3 and 3a-3d, 14:23 PST
item 1; q4-answer.txt 69d3533daa89a1a9 items 1-4). smoke.sh, batch.sh, j1live/j1live.sh and probe/j2probe.sh call it;
no other T2 file reads RK_METER_FILE for a decision (batch.sh's meter() prints the raw file to the log only).

  meter.py read FILE
      One reading: "W F AGE TS R5" (R5 is five_hour.resets_at, or - when absent), or "unreadable: WHY" with exit 1.
  meter.py gate --file F --ledger L --name RUN [--weekly N --weekly-key K] [--no-five] --max-age S --hold-max S
                --five-line P --mult M --five-launch P
      Two readings, PAIR_GAP s apart; GO when both pass, printed "GO W1 W2 F1 F2 TS1 TS2" on stdout, exit 0.
      Otherwise HOLD: re-read every STEP s; after --hold-max s of holding, "STOP WHY" on stdout, exit 3.
      Over the five-hour rule with a resets_at 0 to 5 h + RESET_MARGIN s ahead, the wait runs to resets_at +
      RESET_MARGIN, then the readings are taken again and the hold clock starts over (3a). One such wait per gate:
      a reading over the rule after it holds on the clock like any other. No wait when ceil(M x D) alone is over
      --five-line, since no fresh window could pass. Progress goes to stderr.
      Exit 2: a usage or pins error, or a ledger line that does not parse.
  meter.py end LEDGER RUN          appends "end RUN at=EPOCH" (the launcher's clock after run.sh returned)
  meter.py cap STOP_PCT W0 USD     the 3c cap: (STOP_PCT - W0) x USD in exact decimals, first token the cap, then
                                   its arithmetic
  meter.py spend RUNS CAP RUN...   the 3c sum: total_cost_usd from RUNS/RUN/result.json over the named runs, the
                                   ones this launch ran (void runs included; a run with no readable cost is counted
                                   as unread, as batch.sh's money step skips it); exit 1 when the sum reaches CAP, 0
                                   under it, 2 on an error

A reading passes when (item 1) the file is a JSON object whose ts is an integer, whose rate_limits holds seven_day and
five_hour objects, and whose two used_percentage values are JSON integers (item 4: 91.6, 91.0, true, null and a
string are unreadable, never truncated); (item 2) AGE = now - ts is at or under --max-age, ts not ahead of now, and ts
is later than the last "end" line in the ledger (the reading postdates that run's spend); with --weekly, W is at or
under N; and, without --no-five, the five-hour rule holds (item 3, ruled as 3d): with D measured, F + ceil(M x D) is at
or under --five-line; with no D yet, F is at or under --five-launch.

D is the largest five-hour delta of one run in the ledger. At a gate, the first pair whose two readings pass items 1
and 2 measures the run the ledger's last "gate" line opened, when that run has an "end" line and no "delta" line yet:
max(F1, F2) now minus min(F1, F2) at its gate, counted only when both pairs name the same five_hour.resets_at (a
reset between them leaves the run "unmeasured"). The ledger is append-only and shared by the smoke and the batch
(both write RK_RUNS/t2-meter.txt), so the smoke's runs come first in D and the batch's first gate reads after the
smoke's last run.
"""
import argparse
import json
import math
import os
import re
import sys
import time
from decimal import Decimal
from fractions import Fraction

PAIR_GAP = 20
STEP = 30
RESET_MARGIN = 60
FIVE_HOUR_S = 5 * 3600
RUN = re.compile(r"t2-[gru]([0-9]+)x?")
WORD = re.compile(r"^[A-Za-z0-9._:-]+$")


class Clock:
    def now(self):
        return int(time.time())

    def sleep(self, s):
        if s > 0:
            time.sleep(s)


class Usage(Exception):
    pass


def isint(v):
    return isinstance(v, int) and not isinstance(v, bool)


def read(path, now):
    """One reading: a dict, or a string saying why it is unreadable (item 1, item 4)."""
    try:
        with open(path, "rb") as f:
            d = json.loads(f.read().decode("utf-8"))
    except FileNotFoundError:
        return "no file"
    except (OSError, UnicodeDecodeError, ValueError) as e:
        return "not JSON (%s)" % type(e).__name__
    if not isinstance(d, dict):
        return "not a JSON object"
    ts = d.get("ts")
    if not isint(ts):
        return "ts is %r, not an integer" % (ts,)
    rl = d.get("rate_limits")
    if not isinstance(rl, dict):
        return "no rate_limits object"
    out = {"ts": ts, "age": now - ts}
    for key, short in (("seven_day", "w"), ("five_hour", "f")):
        o = rl.get(key)
        if not isinstance(o, dict):
            return "keyless: no %s object" % key
        p = o.get("used_percentage")
        if not isint(p):
            return "%s.used_percentage is %r, not a JSON integer" % (key, p)
        out[short] = p
    r5 = rl["five_hour"].get("resets_at")
    out["r5"] = r5 if isint(r5) else None
    if ts > now + 2:
        return "ts %d is ahead of now %d" % (ts, now)
    return out


def fmt(r):
    return "%d %d %d %d %s" % (r["w"], r["f"], r["age"], r["ts"], "-" if r["r5"] is None else r["r5"])


def parse_ledger(path):
    """[(kind, name, {k: v})]; a line that does not parse is a Usage error (exit 2)."""
    rows = []
    try:
        with open(path) as f:
            lines = f.read().splitlines()
    except FileNotFoundError:
        return rows
    for i, line in enumerate(lines, 1):
        parts = line.split(" ")
        if len(parts) < 2 or parts[0] not in ("gate", "end", "delta") or not WORD.match(parts[1]):
            raise Usage("ledger %s:%d does not parse: %r" % (path, i, line[:120]))
        kv = {}
        for p in parts[2:]:
            k, eq, v = p.partition("=")
            if not eq or not WORD.match(k) or not WORD.match(v):
                raise Usage("ledger %s:%d does not parse: %r" % (path, i, line[:120]))
            kv[k] = v
        rows.append((parts[0], parts[1], kv))
    return rows


def ledger_state(rows):
    """(after, open_run, D): the last end time; the run whose delta is still owed (its gate kv and end time); D."""
    after = 0
    last_gate = None
    ends, deltas = {}, {}
    d_max = None
    kind, name = "?", "?"
    try:
        for kind, name, kv in rows:
            if kind == "gate":
                _ = (int(kv["f1"]), int(kv["f2"]))  # the delta reads them later; refuse a malformed gate line now
                last_gate = (name, kv)
            elif kind == "end":
                ends[name] = int(kv["at"])
                after = max(after, int(kv["at"]))
            elif kind == "delta":
                deltas[name] = kv
                if "d" in kv:
                    d_max = int(kv["d"]) if d_max is None else max(d_max, int(kv["d"]))
    except (KeyError, ValueError) as e:
        raise Usage("ledger %s line for %s lacks a whole-number field (%s: %s)" % (kind, name, type(e).__name__, e))
    owed = None
    if last_gate and last_gate[0] in ends and last_gate[0] not in deltas:
        owed = (last_gate[0], last_gate[1], ends[last_gate[0]])
    return after, owed, d_max


def append(path, line):
    with open(path, "a") as f:
        f.write(line + "\n")


def say(msg):
    print("  meter: " + msg, file=sys.stderr, flush=True)


def verdict(r, after, a, d_max):
    """None when the reading passes; else (why, five) where five says the five-hour rule failed."""
    if isinstance(r, str):
        return ("unreadable: " + r, False)
    if r["age"] > a.max_age:
        return ("stale: AGE %d s over T2_METER_MAX_AGE_S %d (ts %d)" % (r["age"], a.max_age, r["ts"]), False)
    if r["ts"] <= after:
        return ("not after the previous run's end: ts %d at or before %d" % (r["ts"], after), False)
    if a.weekly is not None and r["w"] > a.weekly:
        return ("weekly %d over %d (%s)" % (r["w"], a.weekly, a.weekly_key), False)
    if not a.no_five:
        if d_max is None:
            if r["f"] > a.five_launch:
                return ("five-hour %d over T2_METER_5H_LAUNCH_PCT %d (no run delta measured yet)"
                        % (r["f"], a.five_launch), True)
        else:
            need = r["f"] + math.ceil(a.mult * d_max)
            if need > a.five_line:
                return ("five-hour %d + ceil(%s x D %d) = %d over T2_METER_5H_PCT %d"
                        % (r["f"], a.mult_s, d_max, need, a.five_line), True)
    return None


def fresh(r, after, a):
    return not isinstance(r, str) and r["age"] <= a.max_age and r["ts"] > after


def gate(a, clock):
    rows = parse_ledger(a.ledger)
    after, owed, d_max = ledger_state(rows)
    say("gate %s: after %d, D %s, %s" % (a.name, after, "none" if d_max is None else d_max,
                                          "weekly line %d (%s)" % (a.weekly, a.weekly_key) if a.weekly is not None
                                          else "no weekly line"))
    hold_since = None
    waited = False
    while True:
        r1 = read(a.file, clock.now())
        v1 = verdict(r1, after, a, d_max)
        r2, v2 = None, None
        if v1 is None or (owed and fresh(r1, after, a)):
            clock.sleep(PAIR_GAP)
            r2 = read(a.file, clock.now())
            v2 = verdict(r2, after, a, d_max)
        if owed and fresh(r1, after, a) and r2 is not None and fresh(r2, after, a):
            assert isinstance(r1, dict) and isinstance(r2, dict)
            name, kv, _ = owed
            r5 = "-" if r2["r5"] is None else str(r2["r5"])
            if kv.get("r5") == r5 and r5 != "-":
                d = max(r1["f"], r2["f"]) - min(int(kv["f1"]), int(kv["f2"]))
                if d >= 0:
                    append(a.ledger, "delta %s d=%d r5=%s" % (name, d, r5))
                    d_max = d if d_max is None else max(d_max, d)
                    say("run %s five-hour delta %d (D now %d)" % (name, d, d_max))
                else:
                    append(a.ledger, "delta %s unmeasured=negative" % name)
                    say("run %s five-hour delta negative; unmeasured" % name)
            else:
                append(a.ledger, "delta %s unmeasured=reset" % name)
                say("run %s unmeasured: five_hour.resets_at %s at its gate, %s now" % (name, kv.get("r5"), r5))
            owed = None
            v1 = verdict(r1, after, a, d_max)
            v2 = verdict(r2, after, a, d_max)
        if v1 is None and v2 is None:
            assert isinstance(r1, dict) and isinstance(r2, dict)
            append(a.ledger, "gate %s ts=%d f1=%d f2=%d w1=%d w2=%d r5=%s at=%d"
                   % (a.name, r2["ts"], r1["f"], r2["f"], r1["w"], r2["w"],
                      "-" if r2["r5"] is None else r2["r5"], clock.now()))
            print("GO %d %d %d %d %d %d" % (r1["w"], r2["w"], r1["f"], r2["f"], r1["ts"], r2["ts"]))
            return 0
        held_on = v1 if v1 is not None else v2
        assert held_on is not None
        why, five = held_on
        bad = r1 if v1 is not None else r2
        now = clock.now()
        nofit = d_max is not None and math.ceil(a.mult * d_max) > a.five_line
        if (five and not waited and not nofit and isinstance(bad, dict) and bad["r5"] is not None
                and 0 < bad["r5"] - now <= FIVE_HOUR_S + RESET_MARGIN):
            wait = bad["r5"] + RESET_MARGIN - now
            say("HOLD %s: %s; waiting %d s to resets_at %d + %d s (the one reset wait of this gate)"
                % (a.name, why, wait, bad["r5"], RESET_MARGIN))
            clock.sleep(wait)
            waited = True
            hold_since = None
            continue
        if five and nofit:
            why += "; ceil(%s x D %d) alone is over %d, so no fresh window can pass" % (a.mult_s, d_max, a.five_line)
        if hold_since is None:
            hold_since = now
        held = now - hold_since
        if held >= a.hold_max:
            print("STOP %s: %s; held %d s of T2_METER_HOLD_MAX_S %d" % (a.name, why, held, a.hold_max))
            return 3
        say("HOLD %s: %s; held %d s of %d, re-read in %d s" % (a.name, why, held, a.hold_max, STEP))
        clock.sleep(STEP)


def spend(runs, cap, names):
    if not os.path.isdir(runs):
        raise Usage("RUNS [%s] is not a directory" % runs)
    if not names:
        raise Usage("spend names no run")
    total, n, unread = Decimal(0), 0, []
    for x in names:
        if not RUN.fullmatch(x):
            raise Usage("run name [%s] is not t2-[gru]N[x]" % x)
        try:
            with open(os.path.join(runs, x, "result.json")) as f:
                c = json.load(f).get("total_cost_usd")
        except (OSError, ValueError, AttributeError):
            c = None
        if isinstance(c, bool) or not isinstance(c, (int, float)) or c < 0 or c != c or c == float("inf"):
            unread.append(x)
            continue
        total += Decimal(repr(c))
        n += 1
    reached = total >= cap
    print("SPEND %s USD over %d runs of this launch (%d without a readable cost%s); 3c cap %s USD: %s"
          % (total, n, len(unread), ": " + " ".join(unread) if unread else "", cap, "REACHED" if reached else "under"))
    return 1 if reached else 0


def num(s, what, kind: type = int):
    """A pins or argument value: int for a whole number, Fraction or Decimal for a decimal ([0-9]+ or [0-9]+.[0-9]+)."""
    pat = r"[0-9]+" if kind is int else r"[0-9]+(\.[0-9]+)?"
    if not isinstance(s, str) or not re.fullmatch(pat, s):
        raise Usage("%s is [%s], not a %s" % (what, s, "whole number" if kind is int else "decimal number"))
    return kind(s)


def main(argv, clock=None):
    clock = clock or Clock()
    if not argv:
        raise Usage("usage: meter.py read FILE | gate ... | end LEDGER RUN | cap STOP W0 USD | spend RUNS CAP RUN...")
    cmd, rest = argv[0], argv[1:]
    if cmd == "read" and len(rest) == 1:
        r = read(rest[0], clock.now())
        if isinstance(r, str):
            print("unreadable: " + r)
            return 1
        print(fmt(r))
        return 0
    if cmd == "end" and len(rest) == 2:
        if not WORD.match(rest[1]):
            raise Usage("run name [%s]" % rest[1])
        parse_ledger(rest[0])
        append(rest[0], "end %s at=%d" % (rest[1], clock.now()))
        return 0
    if cmd == "cap" and len(rest) == 3:
        stop, w0 = num(rest[0], "T2_METER_STOP_PCT"), num(rest[1], "W0")
        usd = num(rest[2], "T2_USD_PER_WEEKLY_PT", Decimal)
        cap = (stop - w0) * usd
        if cap <= 0:
            raise Usage("3c cap (%d - %d) x %s = %s is not above 0" % (stop, w0, rest[2], cap))
        print("%s USD = (T2_METER_STOP_PCT %d - W0 %d) x T2_USD_PER_WEEKLY_PT %s = %d x %s"
              % (cap, stop, w0, rest[2], stop - w0, rest[2]))
        return 0
    if cmd == "spend" and len(rest) >= 3:
        return spend(rest[0], num(rest[1], "the 3c cap", Decimal), rest[2:])
    if cmd == "gate":
        p = argparse.ArgumentParser(prog="meter.py gate", add_help=False)
        for o in ("--file", "--ledger", "--name", "--max-age", "--hold-max", "--five-line", "--mult", "--five-launch"):
            p.add_argument(o, required=True)
        p.add_argument("--weekly")
        p.add_argument("--weekly-key")
        p.add_argument("--no-five", action="store_true")
        try:
            a = p.parse_args(rest)
        except SystemExit:
            raise Usage("gate: bad arguments %r" % (rest,))
        if not WORD.match(a.name):
            raise Usage("run name [%s]" % a.name)
        if (a.weekly is None) != (a.weekly_key is None):
            raise Usage("--weekly and --weekly-key go together")
        if a.weekly_key is not None and not WORD.match(a.weekly_key):
            raise Usage("--weekly-key [%s]" % a.weekly_key)
        a.weekly = None if a.weekly is None else num(a.weekly, a.weekly_key)
        a.max_age = num(a.max_age, "T2_METER_MAX_AGE_S")
        a.hold_max = num(a.hold_max, "T2_METER_HOLD_MAX_S")
        a.five_line = num(a.five_line, "T2_METER_5H_PCT")
        a.five_launch = num(a.five_launch, "T2_METER_5H_LAUNCH_PCT")
        a.mult_s = a.mult
        a.mult = num(a.mult, "T2_METER_5H_MULT", Fraction)
        return gate(a, clock)
    raise Usage("usage: meter.py read FILE | gate ... | end LEDGER RUN | cap STOP W0 USD | spend RUNS CAP RUN...; got %r" % (argv,))


if __name__ == "__main__":
    # Exit 1 is a meaning (read: unreadable; spend: the cap is reached), so any other failure exits 2.
    try:
        rc = main(sys.argv[1:])
    except Usage as e:
        print("meter.py: " + str(e), file=sys.stderr)
        rc = 2
    except Exception as e:
        print("meter.py: %s: %s" % (type(e).__name__, e), file=sys.stderr)
        rc = 2
    sys.exit(rc)
