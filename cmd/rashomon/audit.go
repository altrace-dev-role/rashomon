package main

import "io"

// auditUsage is the usage screen's audit entry (a const of its own, joined in
// verbs.go).
const auditUsage = `  rashomon audit [--days N]      count what you corrected your agent for in
                               the last N days (default 14) of Claude Code
                               sessions on this machine: read here, written
                               nowhere, 0 tokens; a card you can copy

`

// cmdAudit is `rashomon audit` (card SEED1, PF-AU1).
func cmdAudit(args []string, stdout io.Writer) error { return nil }
