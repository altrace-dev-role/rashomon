# STR-11: slot types only, bounded. Each slot's charset or range is archive
# 40.3.8's, and render.New takes a binding of slot-type fields only and runs
# every slot's check. A charset or range drifting from the archive's table, a
# check skipped, or a non-slot field accepted is the whole regression.
m("STR-11 AgentType refuses Claude Code's own agent types again", "internal/steer/render/slot/slot.go",
  "agentTypeRE = &lazyRegexp{pattern: `^[A-Za-z0-9:._-]{1,64}$`}",
  "agentTypeRE = &lazyRegexp{pattern: `^[a-z][a-z0-9_]{0,31}$`}", "TestSTR11b_")
m("STR-11 Model refuses a bracketed model id", "internal/steer/render/slot/slot.go",
  r"modelRE = &lazyRegexp{pattern: `^[A-Za-z0-9._\[\]-]{1,64}$`}",
  r"modelRE = &lazyRegexp{pattern: `^[A-Za-z0-9._-]{1,64}$`}", "TestSTR11b_")
m("STR-11 Exit refuses a negative exit status", "internal/steer/render/slot/slot.go",
  "inRange(\"exit code\", int(e), -maxExit, maxExit)",
  "inRange(\"exit code\", int(e), 0, maxExit)", "TestSTR11b_")
m("STR-11 Minutes stops at a day", "internal/steer/render/slot/slot.go",
  "const maxMinutes = 10_080\n", "const maxMinutes = 1_440\n", "TestSTR11b_")
m("STR-11 Count stops below a million", "internal/steer/render/slot/slot.go",
  "const maxCount = 1_000_000\n", "const maxCount = 999_999\n", "TestSTR11b_")
m("STR-11 RuleID admits dots, underscores and capitals", "internal/steer/render/slot/slot.go",
  "ruleIDRE = &lazyRegexp{pattern: `^[a-z0-9][a-z0-9-]{0,47}$`}",
  "ruleIDRE = &lazyRegexp{pattern: `^[A-Za-z0-9._-]{1,64}$`}", "TestSTR11b_")
m("STR-11 USD admits amounts past 10,000", "internal/steer/render/slot/slot.go",
  "const maxUSD = 10_000\n", "const maxUSD = 9_999_999\n", "TestSTR11b_")
m("STR-11 Gate admits a flag-shaped word", "internal/steer/render/slot/slot.go",
  "\t\tif strings.HasPrefix(w, \"-\") || strings.Trim(w, \".\") == \"\" {\n",
  "\t\tif false {\n", "TestSTR11b_")
m("STR-11 Approved sees a URL scheme only before //", "internal/steer/render/slot/slot.go",
  "*://|(?i:(?:^|", "*://|(?i:x\\bx(?:^|", "TestSTR11b_")
m("STR-11 LessonID admits a capital letter", "internal/steer/render/slot/slot.go",
  "lessonIDRE = &lazyRegexp{pattern: `^[a-z0-9][a-z0-9._:@-]{0,63}$`}",
  "lessonIDRE = &lazyRegexp{pattern: `^[A-Za-z0-9][A-Za-z0-9._:@-]{0,63}$`}", "TestSTR11b_")
m("STR-11 LessonID skips the secret screens", "internal/steer/render/slot/slot.go",
  "\treturn screened(\"lesson id\", string(l))\n", "\treturn nil\n", "TestSTR11b_")
m("STR-11 Enum ignores its table", "internal/steer/render/slot/enum.go",
  "\tif !slices.Contains(enumTables[e.table], e.value) {\n", "\tif !slices.Contains(append(enumTables[e.table], e.value), e.value) {\n", "TestSTR11b_")
m("STR-11 an Enum value brings its own table", "internal/steer/render/slot/enum.go",
  "func NewEnum(table EnumTable, value string) Enum {\n",
  "func NewEnum(table EnumTable, value string) Enum {\n\tenumTables[table] = append(enumTables[table], value)\n", "TestSTR11c_")
m("STR-11 Path admits a '.' segment", "internal/steer/render/slot/path.go",
  "\t\tif seg == \".\" || seg == \"..\" {\n", "\t\tif seg == \"..\" {\n", "TestSTR11b_")
m("STR-11 the list counts {k} from its own paths, not the record", "internal/steer/render/slot/path.go",
  "\tmore := l.more\n", "\tmore := 0\n", "TestSTR11b_")
m("STR-11 New accepts a field that is not a slot type", "internal/steer/render/render.go",
  "\t\t\tf.Type.PkgPath() != slotPkgPath || f.Type.Kind() == reflect.Interface {\n",
  "\t\t\tfalse {\n", "TestSTR11b_")
m("STR-11 New skips the slot checks", "internal/steer/render/render.go",
  "\t\tbound, err := slot.Bind(val)\n", "\t\tbound, err := val, error(nil)\n", "TestSTR11b_")
