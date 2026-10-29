package main

// hookUsage is the usage screen's second part: the entries Claude Code
// invokes and a person never runs. A constant of its own, which usage joins on
// its closing line, so that usage stays under Law 10's limit as the commands a
// person runs grow (card 4.1b); the screen it prints is unchanged.
const hookUsage = `invoked by Claude Code, never by hand:
  rashomon hook [--install ID]   handle one PreToolUse invocation
  rashomon post [--install ID]   handle one PostToolUse invocation
  rashomon probe start [--install ID]
                               handle SessionStart
  rashomon probe end [--install ID]
                               handle SessionEnd
  rashomon recap [--install ID]  handle Stop / StopFailure: print an
                               exception-only line on stdout as a
                               systemMessage when a turn has something worth
                               looking at, and nothing otherwise; never
                               exits non-zero
  rashomon recap pending [--install ID]
                               handle UserPromptSubmit: print that line for
                               the turn that just ended if no recap reached
                               it, as when a refused permission prompt
                               interrupted it; never exits non-zero

--install names the install whose entry is running. An entry belonging to
another install stands down: it records nothing in this environment.
`
