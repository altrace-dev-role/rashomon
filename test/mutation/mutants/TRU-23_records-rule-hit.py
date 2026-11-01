# TRU-23: the inventory's row for a run's records.ndjson says it holds a
# spec_event when a call matched a forbid_command rule: the rule's id, the
# declaring spec file's digest, the hook event and the landed seq (card
# SPEC-RULES-STOP1). The break drops that clause from the row.
m("TRU-23 the records row omits the rule hit it holds", "internal/disclose/disclose.go",
  "; where a call matched a forbid_command rule of the spec it was judged against, a spec_event with the rule's id, the content digest of the spec file that declared it, the hook event and the seq the call's execution landed at; at record scope v5 only,",
  "; at record scope v5 only,", "TestTRU23_RecordsRowNamesTheRuleHit")
