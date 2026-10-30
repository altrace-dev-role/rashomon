package disclose

import "github.com/altrace-dev-role/rashomon/internal/spec"

// specFiles is the inventory's rows for the trust bindings `rashomon spec
// trust` writes (card SPEC-TRUST1) and their replacement. Like learnFiles,
// they sit apart from defaultFiles for Law 10.
func specFiles() []FileRule {
	return []FileRule{
		{
			Base:       "",
			Pattern:    spec.TrustFile,
			Mode:       "0600",
			Purpose:    "the repository specs a person trusted at a terminal, so what each declares counts; an edited file matches none",
			Holds:      "per trusted spec: its project's directory and its path in the clear, its digest, and when it was trusted",
			NeverHolds: "a spec's contents, or anything about a call",
			RemovedBy:  "`rashomon spec untrust`; purge",
			Owner:      "spec",
		},
		{
			Base:       "",
			Pattern:    "." + spec.TrustFile + ".tmp-*",
			Mode:       "0600",
			Purpose:    "spec-trust.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what spec-trust.json is about to hold",
			NeverHolds: "a spec's contents, or anything about a call",
			RemovedBy:  "the write that made it (renamed over spec-trust.json); purge",
			Owner:      "spec",
		},
	}
}
