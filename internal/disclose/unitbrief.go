package disclose

// unitBriefFiles is the inventory's rows for the unit briefs the prompt
// hook writes while the session's content is kept (cmd/rashomon's
// writeUnitBriefs, W-M2.3 hook half, D12), apart from defaultFiles for Law
// 10 as blastReadFiles is. The base and the temp pattern are graph's
// (StoreDir, UnitBriefSubdir, UnitBriefTempPattern), written here as
// literals since disclose does not import graph; TestUB12 holds them to the
// names the writer makes.
func unitBriefFiles() []FileRule {
	const base = "graph/unitbrief/<session>"
	const never = "any word of the prompt, and any file's contents"
	return []FileRule{
		{
			Base:    base,
			Pattern: "x-*.json",
			Mode:    "0600",
			Purpose: "the brief for the first edit in one unit a prompt's selection lies in, written at the prompt " +
				"while the session's content is kept, at most eight a prompt; one file per unit, which a later prompt rewrites",
			Holds: "a version, the unit's id and its first line's key in the repository graph, that line's tier, the " +
				"index's commit, the prompt's id, and for each line its key, its one fact (the files of other units that " +
				"use the unit; a recorded decision's or lesson's id, title, source and the one sentence a brief may carry; " +
				"or the existing files, functions and types offered for reuse) and the counts of what it leaves out",
			NeverHolds: never,
			RemovedBy:  "purge",
			Owner:      "cmd",
		},
		{
			Base:       base,
			Pattern:    ".unitbrief-*.tmp",
			Mode:       "0600",
			Purpose:    "a unit brief being written, renamed over its name once whole; left only when a write stops between its create and its rename",
			Holds:      "what the unit brief it becomes holds",
			NeverHolds: never,
			RemovedBy:  "the writer's rename, or its removal when the write fails; purge",
			Owner:      "cmd",
		},
	}
}
