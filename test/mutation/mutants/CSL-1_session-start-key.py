# COMPACT-SLOT1: steer's probed-version table keys SessionStart's rows by the
# source the call carries (P-TRU-1 2026-11-02 on 2.1.280 and 2.1.316: compact
# passed both halves; startup, resume and clear failed the user's), so a
# lookup by the event alone opens nothing, and every other event ignores a
# source on its call.
C = "internal/steer/client.go"

m("CSL-1 SessionStart keyed by the event alone", C,
  "\t\treturn channelKey{event: event, source: c.StartSource}\n", "\t\treturn channelKey{event: event}\n",
  "TestCSL1_")
m("CSL-1 a source keys every event", C,
  "\treturn channelKey{event: event}\n}\n", "\treturn channelKey{event: event, source: c.StartSource}\n}\n", "TestCSL1_")
m("CSL-1 compact's floor dropped", C,
  '\t{event: string(EventSessionStart), source: "compact"}: {floor: "2.1.280"},\n',
  '\t{event: string(EventSessionStart), source: "compact"}: {},\n', "TestCSL1_")
m("CSL-1 compact's floor below its probe", C,
  '\t{event: string(EventSessionStart), source: "compact"}: {floor: "2.1.280"},\n',
  '\t{event: string(EventSessionStart), source: "compact"}: {floor: "2.1.200"},\n', "TestCSL1_")
