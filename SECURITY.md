# Security Policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub, not in a public issue and not in
a pull request.

Use the repository's Security tab, then Report a vulnerability. That opens a
private advisory visible only to you and the maintainers, where a patch can be
developed and a CVE requested before anything is public.

The setting that exposes that button is private vulnerability reporting, and it
has to be enabled by the repository owner (Settings, Code security and analysis,
Private vulnerability reporting). If the button is not there, the setting is off:
open an issue that says only that you have a security report and are waiting for
a private channel, with no details, and wait for a maintainer to enable it.

There is no security mailbox. A single reporting channel keeps the report, the
fix, and the advisory in one place, and avoids publishing an address that cannot
be rotated.

## What to include

- The version or commit the finding is against.
- What an attacker gains, stated as a concrete outcome.
- The smallest reproduction you have: a payload, a configuration, a test.
- Whether the finding requires an attacker to already have write access to the
  user's home directory.

## Response expectation

- Acknowledgement within three working days.
- An assessment, with a severity and whether it is accepted, within ten working
  days.
- For an accepted finding, a fix or a documented mitigation before the advisory
  is published, and credit in the advisory unless you decline it.

These are intended times, not a contractual SLA.

## Scope

This project installs a `PreToolUse` hook into the user's Claude Code
configuration and writes a local store. The findings that matter most are the
ones that break one of the guarantees the README states:

- **Content reaching disk.** Any path by which prompt text, argument values,
  command strings, tool output, or model responses end up in the store, in a log,
  on stdout, or on stderr. Hook stdout and stderr are written to Claude Code's
  debug log, so this includes panics and tracebacks that carry the payload.
- **Becoming an enforcer.** Any input that causes the handler to exit non-zero,
  and in particular to exit 2, which blocks the user's tool call. An unrecovered
  panic in any goroutine is the expected shape of this bug.
- **Corrupting or destroying the user's configuration.** Any path by which
  `watch` or `detach` loses a foreign hook entry, drops a concurrent edit to
  `~/.claude/settings.json`, or leaves that file in a state that is neither the
  pre-state nor the post-state.
- **Misreported coverage.** Any path by which a run renders as verified when
  records were dropped, or renders a count when the instrumentation could not
  measure itself.
- **Digest reversal.** Any weakness that lets the per-install HMAC key be
  recovered, or lets command strings be recovered from `digest` values.
- **Egress.** Any non-loopback socket opened by any code path.

Out of scope: findings that require an attacker to already hold write access to
the user's home directory or the store; the documented limit of the liveness
probe, which detects hook-system death and cannot detect a recorder that runs and
drops records; results from a scanner with no demonstrated impact.
