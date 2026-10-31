# LU-1: a declaration records, in one bit, that the line's leading cd steps
# were ones hook.Locate does not read (card 5.14b, call schema 9), so the
# report can name an uncredited run under `cd DIR;` as not credited for that
# reason. The key is omitted when false. Each break removes one rule and names
# the tests that hold it.
H = "internal/hook/handle.go"
R = "internal/store/record.go"
K = "internal/store/kinds.go"
A = "internal/report/account.go"
T = "internal/report/text.go"
m("LU-1 an unread fold with a cwd is not recorded unread", H,
  "return s, shape.UnreadCWDDigest(key, cwd, unread), true",
  "return s, shape.UnreadCWDDigest(key, cwd, unread), false",
  "TestLU1_|TestH155_")
m("LU-1 an unread fold with no cwd is not recorded unread", H,
  "			return s, \"\", true\n",
  "			return s, \"\", false\n",
  "TestLU1_")
m("LU-1 a read fold is recorded unread", H,
  "return s, shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs))), false",
  "return s, shape.CWDDigest(key, physicalDir(startDirectory(cwd, dirs))), true",
  "TestLU1_|TestH155_")
m("LU-1 a line with no cd is recorded unread", H,
  "return s, shape.CWDDigest(key, physical), false",
  "return s, shape.CWDDigest(key, physical), true",
  "TestLU1_|TestH155_")
m("LU-1 Capture drops the bit", H,
  "decl.Shape, decl.CWDDigest, decl.LocationUnread = locate(",
  "decl.Shape, decl.CWDDigest, _ = locate(",
  "TestLU1_|TestH155_")
m("LU-1 the key is written when false", R,
  "`json:\"location_unread,omitempty\"`",
  "`json:\"location_unread\"`",
  "TestLU1_TheBitIsOmitted")
m("LU-1 the key is renamed", R,
  "`json:\"location_unread,omitempty\"`",
  "`json:\"location_not_read,omitempty\"`",
  "TestLU1_TheBitIsOmitted")
m("LU-1 the call version stays at 8", K,
  "const CallSchemaVersion = 9",
  "const CallSchemaVersion = 8",
  "TestREC1_|TestAcceptsKind")
m("LU-1 the report counts every uncredited run", A,
  "		if d.LocationUnread {\n			out.UnreadLocation++\n		}\n",
  "		out.UnreadLocation++\n",
  "TestLU1b_")
m("LU-1 the report counts none", A,
  "		if d.LocationUnread {\n			out.UnreadLocation++\n		}\n",
  "",
  "TestLU1b_|TestH155_")
m("LU-1 the report does not name them", T,
  "	if m.UnreadLocation > 0 {\n",
  "	if false {\n",
  "TestLU1b_|TestH155_")
m("LU-1 the report names them with none unread", T,
  "	if m.UnreadLocation > 0 {\n",
  "	if m.Runs > 0 {\n",
  "TestLU1b_")
