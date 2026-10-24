# SP: the spec.json parser (internal/spec): every cap, every closed list, every
# rule kind, every facts constraint and the canonical digest. Each mutant
# names the test and the assertion that kills it, read when the mutant was
# written. The file is Python, so the constants and the closed lists are
# generated from one table each; every row still names its own killer.
PARSE = "internal/spec/parse.go"
WALK = "internal/spec/walk.go"
VET = "internal/spec/vet.go"
LEX = "internal/spec/lex.go"
JCS = "internal/spec/jcs.go"
REGEX = "internal/spec/regex.go"
SPEC = "internal/spec/spec.go"
FILE = "internal/spec/file.go"
ERR = "internal/spec/error.go"

# --- the caps as constants. Killed by TestSP2_CapsArePinned/<Name>, which
# compares each to the archive's number written as a literal; the behaviour
# mutants below move the comparisons.
for name, anchor, new in [
    ("MaxBytes", "\tMaxBytes               = 256 << 10\n", "\tMaxBytes               = 256 << 11\n"),
    ("MaxGates", "\tMaxGates               = 32 // including ci_commands\n", "\tMaxGates               = 33 // including ci_commands\n"),
    ("MaxRules", "\tMaxRules               = 64\n", "\tMaxRules               = 65\n"),
    ("MaxFacts", "\tMaxFacts               = 64\n", "\tMaxFacts               = 65\n"),
    ("MaxPatternBytes", "\tMaxPatternBytes        = 512\n", "\tMaxPatternBytes        = 513\n"),
    ("MaxPatternInstructions", "\tMaxPatternInstructions = 2000\n", "\tMaxPatternInstructions = 2001\n"),
    ("MaxSpecInstructions", "\tMaxSpecInstructions    = 20000\n", "\tMaxSpecInstructions    = 20001\n"),
    ("MaxRepeat", "\tMaxRepeat              = 100\n", "\tMaxRepeat              = 101\n"),
]:
    m(f"SP cap {name} moves", SPEC, anchor, new, f"TestSP2_CapsArePinned/^{name}$")
for name, anchor, new in [
    ("MaxWhyRunes", "\tMaxWhyRunes = 300\n", "\tMaxWhyRunes = 301\n"),
    ("MaxAlternativeRunes", "\tMaxAlternativeRunes = 160\n", "\tMaxAlternativeRunes = 161\n"),
    ("MaxIDBytes", "\tMaxIDBytes = 48\n", "\tMaxIDBytes = 49\n"),
    ("MaxArgvWords", "\tMaxArgvWords = 32\n", "\tMaxArgvWords = 33\n"),
    ("MaxAliases", "\tMaxAliases = 8\n", "\tMaxAliases = 9\n"),
    ("MaxListItems", "\tMaxListItems = 64\n", "\tMaxListItems = 65\n"),
    ("MaxWordBytes", "\tMaxWordBytes = 256\n", "\tMaxWordBytes = 257\n"),
    ("MaxFlagValueKeys", "\tMaxFlagValueKeys = 16\n", "\tMaxFlagValueKeys = 17\n"),
    ("MaxExitCodes", "\tMaxExitCodes = 16\n", "\tMaxExitCodes = 17\n"),
    ("MaxFactValueBytes", "\tMaxFactValueBytes = 128\n", "\tMaxFactValueBytes = 129\n"),
    ("MaxDepth", "\tMaxDepth = 16\n", "\tMaxDepth = 17\n"),
    ("MaxNodes", "\tMaxNodes = 20000\n", "\tMaxNodes = 20001\n"),
]:
    m(f"SP cap {name} moves", FILE, anchor, new, f"TestSP2_CapsArePinned/^{name}$")

# --- the caps as comparisons: the boundary is inclusive, so a value at the cap
# is accepted and one over is refused. Killed by the named TestSP2 row.
m("SP file at exactly 256 KiB is refused", PARSE, "\tif len(data) > MaxBytes {\n", "\tif len(data) >= MaxBytes {\n",
  "TestSP2_OverLimitRefused/^file_bytes$")  # ok document is sized(MaxBytes): must be accepted
m("SP an oversize file is digested whole", PARSE, "rawDigest(data[:MaxBytes+1])", "rawDigest(data)",
  "TestSP7_OversizeDigestIgnoresTheTail")  # b has a 5000-byte tail: digests must match a
m("SP 32 gates is refused", PARSE, "\tcase len(gates) > MaxGates:\n", "\tcase len(gates) >= MaxGates:\n",
  "TestSP2_OverLimitRefused/^gates$")
m("SP ci_commands do not count against the gate cap", PARSE, "\tcase len(gates)+len(ci) > MaxGates:\n", "\tcase len(ci) > MaxGates:\n",
  "TestSP2_OverLimitRefused/^gates_plus_ci_commands$")  # 32 gates + 1 ci must be refused at /ci_commands
m("SP 31 gates and 1 ci_command is refused", PARSE, "\tcase len(gates)+len(ci) > MaxGates:\n", "\tcase len(gates)+len(ci) >= MaxGates:\n",
  "TestSP2_OverLimitRefused/^gates_plus_ci_commands$")  # the ok document is 31 + 1
m("SP 64 rules is refused", PARSE, "\tcase len(rules) > MaxRules:\n", "\tcase len(rules) >= MaxRules:\n",
  "TestSP2_OverLimitRefused/^rules$")
m("SP the 65th rule is accepted", PARSE, "\tcase len(rules) > MaxRules:\n", "\tcase len(rules) > MaxRules+1:\n",
  "TestSP2_OverLimitRefused/^rules$")
m("SP 64 facts is refused", PARSE, "\tcase len(facts) > MaxFacts:\n", "\tcase len(facts) >= MaxFacts:\n",
  "TestSP2_OverLimitRefused/^facts$")
m("SP the 65th fact is accepted", PARSE, "\tcase len(facts) > MaxFacts:\n", "\tcase len(facts) > MaxFacts+1:\n",
  "TestSP2_OverLimitRefused/^facts$")
m("SP a 512-byte pattern is refused", REGEX, "\tif len(src) > MaxPatternBytes {\n", "\tif len(src) >= MaxPatternBytes {\n",
  "TestSP2_OverLimitRefused/^pattern_bytes$")
m("SP a 513-byte pattern is parsed", REGEX, "\tif len(src) > MaxPatternBytes {\n", "\tif len(src) > MaxPatternBytes+1 {\n",
  "TestSP5_RegexCapsCountInstructions")  # the 513 'a' pattern is a syntax-valid length error in SP2; a 513 '(' row is the one that parses
m("SP a 2000-instruction pattern is refused", REGEX, "\tif len(prog.Inst) > MaxPatternInstructions {\n", "\tif len(prog.Inst) >= MaxPatternInstructions {\n",
  "TestSP5_RegexCapsCountInstructions/^instructions_per_pattern$")
m("SP a 2001-instruction pattern is accepted", REGEX, "\tif len(prog.Inst) > MaxPatternInstructions {\n", "\tif len(prog.Inst) > MaxPatternInstructions+1 {\n",
  "TestSP5_RegexCapsCountInstructions/^instructions_per_pattern$")
m("SP the 20,000 total is exclusive", WALK, "p.insts > MaxSpecInstructions", "p.insts >= MaxSpecInstructions",
  "TestSP5_RegexCapsCountInstructions")  # ten patterns of exactly 2,000 must pass
m("SP the spec total is never checked", WALK, "p.insts > MaxSpecInstructions", "p.insts < 0",
  "TestSP5_RegexCapsCountInstructions")  # the eleventh pattern must cross at /rules/10/pattern
m("SP instructions are counted after no simplification", REGEX, "syntax.Compile(re.Simplify())", "syntax.Compile(re)",
  "TestSP5_RegexCapsCountInstructions/^instructions_per_pattern$")  # a{100} is one repeat node unexpanded
m("SP a repeat of 101 as a maximum is allowed", REGEX, "\t\tif re.Min > MaxRepeat || re.Max > MaxRepeat {\n", "\t\tif re.Min > MaxRepeat {\n",
  "TestSP5_RegexCapsCountInstructions/^repeat_range_maximum$")
m("SP a repeat of 101 as a minimum is allowed", REGEX, "\t\tif re.Min > MaxRepeat || re.Max > MaxRepeat {\n", "\t\tif re.Max > MaxRepeat {\n",
  "TestSP5_RegexCapsCountInstructions/^repeat_minimum_with_no_maximum$")
m("SP a repeat of 100 is refused", REGEX, "\t\tif re.Min > MaxRepeat || re.Max > MaxRepeat {\n", "\t\tif re.Min >= MaxRepeat || re.Max >= MaxRepeat {\n",
  "TestSP5_RegexCapsCountInstructions/^repeat_count$")
m("SP a nested counted repeat is allowed", REGEX, "\t\tif inside {\n", "\t\tif false {\n",
  "TestSP5_RegexCapsCountInstructions/^nested_counted_repeat$")
m("SP only the first level of nesting is seen", REGEX, "\tfor _, sub := range re.Sub {\n\t\tif r := repeatReason(sub, inside); r != \"\" {", "\tfor _, sub := range re.Sub[:min(len(re.Sub), 1)] {\n\t\tif r := repeatReason(sub, inside); r != \"\" {",
  "TestSP5_RegexCapsCountInstructions/^nested_counted_repeat,_deep$")  # the nest sits in the second branch of an alternation
m("SP Go's own repeat ceiling is a syntax error", REGEX, "se.Code == syntax.ErrInvalidRepeatSize", "se.Code == syntax.ErrMissingParen",
  "TestSP5_RegexCapsCountInstructions/^repeat_past_Go's_ceiling$")
m("SP patterns are read as POSIX", REGEX, "syntax.Parse(src, syntax.Perl)", "syntax.Parse(src, syntax.POSIX)",
  "TestSP1_ParseWithinLimits")  # the example's \s is a Perl class
m("SP a gate's line regexes are never counted", WALK, "\tinsts, reason := checkRegex(s)\n", "\tinsts, reason := 0, Reason(\"\")\n",
  "TestSP5_RegexCapsCountInstructions/^success_line$")
m("SP the limit of a too-long pattern is dropped", WALK, "\t\tp.fail(reason, MaxPatternBytes)\n", "\t\tp.fail(reason, 0)\n",
  "TestSP2_OverLimitRefused/^pattern_bytes$")  # asserts Limit == MaxPatternBytes
m("SP why counts bytes, not runes", PARSE, "utf8.RuneCountInString(n.str) > r.cap", "len(n.str) > r.cap",
  "TestSP2_OverLimitRefused/^why_runes$")  # 300 two-byte runes must pass
m("SP why has no cap", PARSE, "\twhyRule   = strRule{cap: MaxWhyRunes, runes: true}\n", "\twhyRule   = strRule{runes: true}\n",
  "TestSP2_OverLimitRefused/^why_runes$")
m("SP hard_control shares no cap with why", WALK, "\tr.HardControl, _ = p.strAt(f, \"hard_control\", true, whyRule)\n", "\tr.HardControl, _ = p.strAt(f, \"hard_control\", true, strRule{})\n",
  "TestSP2_OverLimitRefused/^hard_control_runes$")
m("SP alternative has the why cap", WALK, "\tr.Alternative, _ = p.strAt(f, \"alternative\", false, altRule)\n", "\tr.Alternative, _ = p.strAt(f, \"alternative\", false, whyRule)\n",
  "TestSP2_OverLimitRefused/^alternative_runes$")  # 161..300 runes must be refused
m("SP a fact's why has no cap", WALK, "\tft.Why, _ = p.strAt(f, \"why\", false, whyRule)\n", "\tft.Why, _ = p.strAt(f, \"why\", false, strRule{})\n",
  "TestSP2_OverLimitRefused/^fact_why_runes$")
m("SP a list may hold one more than its cap", PARSE, "\tcase len(n.vals) > max:\n", "\tcase len(n.vals) > max+1:\n",
  "TestSP2_OverLimitRefused/^gate_argv_words$")
m("SP a list at its cap is refused", PARSE, "\tcase len(n.vals) > max:\n", "\tcase len(n.vals) >= max:\n",
  "TestSP2_OverLimitRefused/^gate_argv_words$")
m("SP aliases at the cap are refused", WALK, "\tcase len(n.vals) > MaxAliases:\n", "\tcase len(n.vals) >= MaxAliases:\n",
  "TestSP2_OverLimitRefused/^aliases$")
m("SP the ninth alias is accepted", WALK, "\tcase len(n.vals) > MaxAliases:\n", "\tcase len(n.vals) > MaxAliases+1:\n",
  "TestSP2_OverLimitRefused/^aliases$")
m("SP flag_values keys at the cap are refused", WALK, "\tcase len(n.keys) > MaxFlagValueKeys:\n", "\tcase len(n.keys) >= MaxFlagValueKeys:\n",
  "TestSP2_OverLimitRefused/^flag_values_keys$")
m("SP exit_codes at the cap are refused", WALK, "\tcase len(n.keys) > MaxExitCodes:\n", "\tcase len(n.keys) >= MaxExitCodes:\n",
  "TestSP2_OverLimitRefused/^exit_codes$")
m("SP a word at its byte cap is refused", PARSE, "len(n.str) > r.cap,", "len(n.str) >= r.cap,",
  "TestSP2_OverLimitRefused/^word_bytes$")
m("SP a flag_values key has no length cap", WALK, "\t\tcase len(k) > MaxWordBytes:\n", "\t\tcase false:\n",
  "TestSP6_StructureErrors")  # a 257-byte key must be too_long
m("SP a fact value may be one byte longer", PARSE, "\tvalueRule = strRule{cap: MaxFactValueBytes, vet: vetFactValue}\n", "\tvalueRule = strRule{cap: MaxFactValueBytes + 1, vet: vetFactValue}\n",
  "TestSP2_OverLimitRefused/^fact_value_bytes$")

# --- ids: charset, length, uniqueness.
m("SP an id may be 49 bytes", VET, "\tif len(s) > MaxIDBytes {\n\t\treturn ReasonID\n\t}\n\tfor i := 0; i < len(s); i++ {\n\t\tc := s[i]\n\t\tif !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {",
  "\tfor i := 0; i < len(s); i++ {\n\t\tc := s[i]\n\t\tif !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {",
  "TestSP2_IDCharsetAndUniqueness")  # a 49-byte id must be refused at each of the three pointers
m("SP an id may start with a dot", VET, "'0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID", "'0' <= c && c <= '9' || (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID",
  "TestSP2_IDCharsetAndUniqueness")  # "-a", ".a", "_a" must be refused
m("SP an id may be upper case", VET, "if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID",
  "if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID",
  "TestSP2_IDCharsetAndUniqueness")  # "A" and "aB" must be refused
m("SP an id may not contain a dot", VET, "i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID", "i > 0 && (c == '_' || c == '-')) {\n\t\t\treturn ReasonID",
  "TestSP2_IDCharsetAndUniqueness")  # "a.b" must be accepted
m("SP an id may not contain an underscore", VET, "i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID", "i > 0 && (c == '.' || c == '-')) {\n\t\t\treturn ReasonID",
  "TestSP2_IDCharsetAndUniqueness")  # "a_b" must be accepted
m("SP an id may not contain a hyphen", VET, "i > 0 && (c == '.' || c == '_' || c == '-')) {\n\t\t\treturn ReasonID", "i > 0 && (c == '.' || c == '_')) {\n\t\t\treturn ReasonID",
  "TestSP2_IDCharsetAndUniqueness")  # "a-b" must be accepted
m("SP a repeated id is accepted", PARSE, "\t\tif _, dup := seen[id]; dup {\n", "\t\tif _, dup := seen[id]; dup && false {\n",
  "TestSP2_IDCharsetAndUniqueness")  # /gates/1/id, /rules/1/id, /facts/1/id duplicate_id
m("SP rules share the gates' id space", PARSE, "\tseen = map[string]struct{}{}\n\tp.push(\"rules\")\n", "\tp.push(\"rules\")\n",
  "TestSP2_IDCharsetAndUniqueness")  # a rule named g beside a gate named g must be accepted

# --- the closed lists. Each loses a member; killed by the accept loop of
# TestSP2_ClosedEnumsAndIDs and by its list-equality assertion.
for name, old, new in [
    ("GateKinds", 'GateKinds = []string{"test", "build", "lint", "typecheck", "scan", "custom"}', 'GateKinds = []string{"test", "build", "lint", "typecheck", "scan"}'),
    ("GateMatches", 'GateMatches = []string{"prefix", "exact"}', 'GateMatches = []string{"exact", "prefix"}'),
    ("ExitStates", 'ExitStates = []string{"pass", "empty_run", "fail"}', 'ExitStates = []string{"pass", "fail"}'),
    ("Modes", 'Modes = []string{"modify", "delete", "create"}', 'Modes = []string{"modify", "delete"}'),
    ("Points", 'Points = []string{"commit", "push", "pr", "done", "subtask"}', 'Points = []string{"commit", "push", "pr", "done"}'),
    ("FactKinds", 'FactKinds = []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name", "sign_in_step"}',
     'FactKinds = []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name"}'),
    ("TargetKinds", 'TargetKinds = []string{"aws_account", "aws_region", "endpoint_host", "kube_context", "gcp_project", "azure_subscription"}',
     'TargetKinds = []string{"aws_account", "aws_region", "endpoint_host", "kube_context", "gcp_project"}'),
    ("SQLPrograms", 'SQLPrograms = []string{"psql", "mysql", "sqlite3", "cqlsh", "clickhouse-client"}', 'SQLPrograms = []string{"psql", "mysql", "sqlite3", "cqlsh"}'),
]:
    m(f"SP closed list {name} loses a member", FILE, old, new, "TestSP2_ClosedEnumsAndIDs")
m("SP a word outside a closed list is accepted", PARSE, "\t\tif !slices.Contains(set, s) {\n", "\t\tif !slices.Contains(set, s) && false {\n",
  "TestSP2_ClosedEnumsAndIDs/^gate_kind$")  # "deploy" must be refused at /gates/0/kind
m("SP an evidence kind outside the gate kinds is accepted", VET, "\t\tif !slices.Contains(GateKinds, k) {\n", "\t\tif false && k == \"\" {\n",
  "TestSP2_ClosedEnumsAndIDs/^evidence_kind$")
m("SP a rule kind outside the five is accepted by name", WALK, "kind, _ := p.strAt(f, \"kind\", true, in(ruleKindNames()))", "kind, _ := p.strAt(f, \"kind\", true, strRule{})",
  "TestSP2_ClosedEnumsAndIDs")  # "forbid_everything" must be refused at /rules/0/kind
m("SP version 2 passes", PARSE, "\tcase v.num != 1:\n", "\tcase v.num < 1:\n", "TestSP2_ClosedEnumsAndIDs")  # {"rashomon_spec":2} version
m("SP version 0 passes", PARSE, "\tcase v.num != 1:\n", "\tcase v.num > 1:\n", "TestSP2_ClosedEnumsAndIDs")  # {"rashomon_spec":0} version
m("SP a string version passes", PARSE, "\tcase v.kind != kInt:\n", "\tcase v.kind != kInt && v.kind != kString:\n",
  "TestSP2_ClosedEnumsAndIDs")  # {"rashomon_spec":"1"} must be wrong_type
m("SP a missing version passes", PARSE, "\tcase !ok:\n\t\tp.failAt(\"rashomon_spec\", ReasonRequired, 0)\n", "\tcase !ok:\n",
  "TestSP2_ClosedEnumsAndIDs")  # {} must be required_missing
m("SP exit codes above 255 pass", WALK, "code < 0 || code > 255 ||", "code < 0 || code > 256 ||", "TestSP12_WrongTypesAtEveryField")  # "256"
m("SP negative exit codes pass", WALK, "code < 0 || code > 255 ||", "code < -1 || code > 255 ||", "TestSP12_WrongTypesAtEveryField")  # "-1"
m("SP exit codes with a leading zero pass", WALK, " || strconv.Itoa(code) != k {", " {", "TestSP12_WrongTypesAtEveryField")  # "07"

# --- unknown keys: the shared refusal, then each level's own key set.
m("SP an unknown key is accepted at every level", PARSE, "\t\tif !slices.Contains(allowed, k) {\n", "\t\tif !slices.Contains(allowed, k) && false {\n",
  "TestSP3_UnknownKeyRefusedAtEveryLevel")
for level, old, new, row in [
    ("top", '\ttopKeys    = []string{"rashomon_spec", "gates", "ci_commands", "rules", "facts"}\n', '\ttopKeys    = []string{"rashomon_spec", "gates", "ci_commands", "rules", "facts", "bogus"}\n', "top"),
    ("gate", '"empty_run_line", "exit_codes"}\n', '"empty_run_line", "exit_codes", "bogus"}\n', "gate"),
    ("command", '\tcmdKeys    = []string{"argv", "flags", "require_flags", "flag_values", "sql"}\n', '\tcmdKeys    = []string{"argv", "flags", "require_flags", "flag_values", "sql", "bogus"}\n', "command"),
    ("fact", '"mechanism", "why"}\n', '"mechanism", "why", "bogus"}\n', "fact"),
    ("fact target", '\ttargetKeys = []string{"kind", "id"}\n', '\ttargetKeys = []string{"kind", "id", "bogus"}\n', "fact_target"),
    ("rule", '\truleCommon = []string{"id", "kind", "why", "hard_control", "alternative"}\n', '\truleCommon = []string{"id", "kind", "why", "hard_control", "alternative", "bogus"}\n', "rule"),
]:
    m(f"SP {level} objects accept an unknown key", PARSE, old, new, f"TestSP3_UnknownKeyRefusedAtEveryLevel/^{row}$")
m("SP keys match case-insensitively", PARSE, "\t\tif !slices.Contains(allowed, k) {\n", "\t\tif !slices.ContainsFunc(allowed, func(a string) bool { return strings.EqualFold(a, k) }) {\n",
  "TestSP3_UnknownKeyRefusedAtEveryLevel/^case_variant$")
IMPORTS["SP keys match case-insensitively"] = (PARSE, "import (\n\t\"crypto/sha256\"\n", "import (\n\t\"strings\"\n\t\"crypto/sha256\"\n")
m("SP a duplicate key takes the last value", LEX, "\t\tif _, dup := seen[key]; dup {\n", "\t\tif _, dup := seen[key]; dup && false {\n",
  "TestSP3_UnknownKeyRefusedAtEveryLevel")  # {"rashomon_spec":1,"rashomon_spec":1} duplicate_key

# --- defaults.
m("SP required defaults to false", WALK, "\tyes := true\n\tg.Required = &yes\n", "\tyes := false\n\tg.Required = &yes\n",
  "TestSP4_RequiredDefaultsTrue/^absent$")
m("SP an explicit required is ignored", WALK, "\t\tyes = rn.flag\n", "\t\t_ = rn.flag\n", "TestSP4_RequiredDefaultsTrue/^explicit_false$")
m("SP a non-boolean required is accepted", WALK, "\t\tif rn.kind != kBool {\n", "\t\tif false {\n", "TestSP4_RequiredDefaultsTrue")  # "yes" and null must be wrong_type
m("SP match defaults to exact", WALK, "\t\tg.Match = GateMatches[0]\n", "\t\tg.Match = GateMatches[1]\n", "TestSP4_RequiredDefaultsTrue")
m("SP dir defaults to empty", WALK, "\t\tg.Dir = \".\"\n", "\t\tg.Dir = \"\"\n", "TestSP4_RequiredDefaultsTrue")
m("SP protect_path modes default to none", WALK, "\t\t\tr.Modes = slices.Clone(Modes)\n", "\t\t\tr.Modes = nil\n", "TestSP4_RequiredDefaultsTrue")
m("SP an unknown trust is kept", PARSE, "\ttrust = Lowest(trust, Managed) // an unknown trust is untrusted\n", "\t_ = Lowest\n",
  "TestSP1_ParseWithinLimits")  # trust "bogus" must come back untrusted

# --- lexical: integers only, strings, nesting, values, UTF-8.
m("SP an exponent is an integer", LEX, "(l.data[l.pos] == '.' || l.data[l.pos] == 'e' || l.data[l.pos] == 'E')", "(l.data[l.pos] == '.' || l.data[l.pos] == 'E')",
  "TestSP6_HostileBytes/^exponent$")
m("SP a fraction is an integer", LEX, "(l.data[l.pos] == '.' || l.data[l.pos] == 'e' || l.data[l.pos] == 'E')", "(l.data[l.pos] == 'e' || l.data[l.pos] == 'E')",
  "TestSP6_HostileBytes/^float$")
m("SP an integer beyond 2^53 is accepted", LEX, "const maxSafeInt = 1<<53 - 1\n", "const maxSafeInt = 1<<62 - 1\n", "TestSP6_HostileBytes/^integer_past_2.53$")
m("SP the 2^53 bound is off by one", LEX, "const maxSafeInt = 1<<53 - 1\n", "const maxSafeInt = 1 << 53\n", "TestSP6_HostileBytes/^negative_past_2.53$")
m("SP a negative integer beyond 2^53 is accepted", LEX, "v > maxSafeInt || v < -maxSafeInt", "v > maxSafeInt", "TestSP6_HostileBytes/^negative_past_2.53$")
m("SP a lone surrogate escape is accepted", LEX, "\t\t\tif !ok || low < 0xDC00 || low > 0xDFFF {\n", "\t\t\tif !ok {\n", "TestSP6_HostileBytes/^high_then_not_low$")
m("SP a low surrogate may open a pair", LEX, "\t\t\tif r < 0xDC00 && l.pos+1 < len(l.data)", "\t\t\tif l.pos+1 < len(l.data)", "TestSP6_HostileBytes/^low_then_low$")
m("SP objects nest one level deeper", LEX, "func (l *lexer) object(depth int) node {\n\tif depth > MaxDepth {", "func (l *lexer) object(depth int) node {\n\tif depth > MaxDepth+1 {", "TestSP6_StructureErrors")
m("SP arrays nest one level deeper", LEX, "func (l *lexer) array(depth int) node {\n\tif depth > MaxDepth {", "func (l *lexer) array(depth int) node {\n\tif depth > MaxDepth+1 {", "TestSP6_HostileBytes")
m("SP objects at the depth cap are refused", LEX, "func (l *lexer) object(depth int) node {\n\tif depth > MaxDepth {", "func (l *lexer) object(depth int) node {\n\tif depth >= MaxDepth {", "TestSP6_StructureErrors")
m("SP arrays at the depth cap are refused", LEX, "func (l *lexer) array(depth int) node {\n\tif depth > MaxDepth {", "func (l *lexer) array(depth int) node {\n\tif depth >= MaxDepth {", "TestSP6_HostileBytes")
m("SP the value cap is exclusive", LEX, "l.nodes++; l.nodes > MaxNodes", "l.nodes++; l.nodes >= MaxNodes", "TestSP6_HostileBytes")
m("SP the value cap never fires", LEX, "l.nodes++; l.nodes > MaxNodes", "l.nodes++; l.nodes < 0", "TestSP6_HostileBytes")
m("SP invalid UTF-8 is accepted", PARSE, "\tif !utf8.Valid(data) {\n", "\tif !utf8.Valid(data) && false {\n", "TestSP6_HostileBytes/^invalid_UTF-8$")
m("SP a raw control character in a string is accepted", LEX, "\t\tcase c < 0x20:\n\t\t\tl.fail(ReasonSyntax, 0)\n", "\t\tcase c < 0x20 && false:\n\t\t\tl.fail(ReasonSyntax, 0)\n", "TestSP6_HostileBytes/^raw_newline_in_string$")
m("SP trailing data after the document is accepted", LEX, "\t\tif l.pos != len(l.data) {\n", "\t\tif l.pos != len(l.data) && false {\n", "TestSP6_HostileBytes/^trailing_data$")
m("SP a control character in text is accepted", VET, "\t\tif unicode.IsControl(r) || r >= 0x202A", "\t\tif r >= 0x202A", "TestSP6_HostileBytes/^control_char_in_why$")
m("SP a bidi override is accepted", VET, "r >= 0x202A && r <= 0x202E", "r >= 0x202B && r <= 0x202E", "TestSP6_HostileBytes/^bidi_embedding_in_why$")
m("SP the last bidi override is accepted", VET, "r >= 0x202A && r <= 0x202E", "r >= 0x202A && r <= 0x202D", "TestSP6_HostileBytes/^bidi_override_in_why$")
m("SP the first bidi isolate is accepted", VET, "r >= 0x2066 && r <= 0x2069", "r >= 0x2067 && r <= 0x2069", "TestSP6_HostileBytes/^bidi_isolate_in_why$")
m("SP the last bidi isolate is accepted", VET, "r >= 0x2066 && r <= 0x2069", "r >= 0x2066 && r <= 0x2068", "TestSP6_HostileBytes/^bidi_isolate_pop_in_why$")
m("SP text next to a bidi range is refused", VET, "r >= 0x202A && r <= 0x202E || r >= 0x2066", "r >= 0x2029 && r <= 0x202F || r >= 0x2066", "TestSP6_HostileBytes")  # U+2029 and U+202F must be accepted
m("SP an argv word may hold a space", VET, "\tif strings.IndexFunc(s, unicode.IsSpace) >= 0 {\n", "\tif strings.IndexFunc(s, unicode.IsSpace) >= 0 && false {\n", "TestSP6_HostileBytes/^space_in_an_argv_word$")

# --- the error: pointer, digest, closed text.
m("SP an invalid spec loses its canonical digest", PARSE, "p.err.Path, p.err.Digest, p.err.Canonical = path, digest, true", "p.err.Path, p.err.Digest, p.err.Canonical = path, rawDigest(data), false",
  "TestSP7_ErrorCarriesTheDigest")
m("SP a lexical error is keyed by the canonical form", PARSE, "\t\tlerr.Path, lerr.Digest = path, rawDigest(data)\n", "\t\tlerr.Path, lerr.Digest = path, DigestPrefix\n", "TestSP7_ErrorCarriesTheDigest")
m("SP a key of 49 bytes shows in the pointer", LEX, "\tif len(s) == 0 || len(s) > MaxIDBytes {\n", "\tif len(s) == 0 {\n", "TestSP7_ErrorTextIsBounded/^long_key$")
m("SP a key of 48 bytes is hidden", LEX, "\tif len(s) == 0 || len(s) > MaxIDBytes {\n", "\tif len(s) == 0 || len(s) >= MaxIDBytes {\n", "TestSP7_ErrorTextIsBounded/^48_byte_key$")
m("SP any key shows in the pointer", LEX, "\t\tif !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false\n", "\t\tif false && c == 0 {\n\t\t\treturn false\n", "TestSP7_ErrorTextIsBounded/^key_with_spaces$")
m("SP a dot is not a safe key character", LEX, "'0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "'0' <= c && c <= '9' || c == '_' || c == '-') {\n\t\t\treturn false", "TestSP7_ErrorTextIsBounded/^short_key$")
m("SP an underscore is not a safe key character", LEX, "'0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "'0' <= c && c <= '9' || c == '.' || c == '-') {\n\t\t\treturn false", "TestSP7_ErrorTextIsBounded/^short_key$")
m("SP a hyphen is not a safe key character", LEX, "'0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "'0' <= c && c <= '9' || c == '.' || c == '_') {\n\t\t\treturn false", "TestSP7_ErrorTextIsBounded/^short_key$")
m("SP a digit is not a safe key character", LEX, "'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "'A' <= c && c <= 'Z' || c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "TestSP7_ErrorTextIsBounded/^short_key$")
m("SP an upper-case letter is not a safe key character", LEX, "if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {", "if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {", "TestSP7_ErrorTextIsBounded/^upper-case_key$")
m("SP the error text drops its limit", ERR, "\tif e.Limit > 0 {\n", "\tif e.Limit > 1<<30 {\n", "TestSP7_ErrorCarriesTheDigest")

# --- the canonical form (RFC 8785).
m("SP keys sort by code point, not UTF-16 unit", JCS, "slices.Compare(units[a], units[b])", "strings.Compare(keys[a], keys[b])", "TestSP11_JCSSortsByUTF16")
IMPORTS["SP keys sort by code point, not UTF-16 unit"] = (JCS, "import (\n\t\"slices\"\n", "import (\n\t\"slices\"\n\t\"strings\"\n")
m("SP keys are not sorted", JCS, "\tidx := make([]int, len(keys))\n", "\tidx := make([]int, len(keys))\n\tif len(keys) > 0 {\n\t\tfor i := range idx {\n\t\t\tidx[i] = i\n\t\t}\n\t\treturn idx\n\t}\n", "TestSP11_JCSSortsByUTF16")
m("SP a space is escaped", JCS, "\t\tcase c < 0x20:\n", "\t\tcase c <= 0x20:\n", "TestSP11_JCSStrings")
m("SP DEL is escaped", JCS, "\t\tcase c < 0x20:\n", "\t\tcase c < 0x20 || c == 0x7f:\n", "TestSP11_JCSStrings")
m("SP backspace is written as a unicode escape", JCS, "\t\tcase c == '\\b':\n\t\t\tdst = append(dst, '\\\\', 'b')\n", "", "TestSP11_JCSStrings")
m("SP form feed is written as a unicode escape", JCS, "\t\tcase c == '\\f':\n\t\t\tdst = append(dst, '\\\\', 'f')\n", "", "TestSP11_JCSStrings")
m("SP a quote is not escaped", JCS, "\t\tcase c == '\"' || c == '\\\\':\n", "\t\tcase c == '\\\\':\n", "TestSP11_JCSStrings")
m("SP a backslash is not escaped", JCS, "\t\tcase c == '\"' || c == '\\\\':\n", "\t\tcase c == '\"':\n", "TestSP11_JCSStrings")
m("SP unicode escapes are upper case", JCS, "const hexDigits = \"0123456789abcdef\"", "const hexDigits = \"0123456789ABCDEF\"", "TestSP11_JCSStrings")
m("SP non-ASCII runes are escaped", JCS, "\t\tdefault:\n\t\t\t_, size := utf8.DecodeRuneInString(s[i:])\n\t\t\tdst = append(dst, s[i:i+size]...)\n", "\t\tdefault:\n\t\t\t_, size := utf8.DecodeRuneInString(s[i:])\n\t\t\tdst = append(dst, '?')\n", "TestSP11_JCSStrings")
m("SP negative integers lose their sign", JCS, "\t\treturn strconv.AppendInt(dst, n.num, 10)\n", "\t\treturn strconv.AppendUint(dst, uint64(max(n.num, -n.num)), 10)\n", "TestSP11_JCSScalars")
m("SP a false is written as true", JCS, "\t\treturn strconv.AppendBool(dst, n.flag)\n", "\t\treturn strconv.AppendBool(dst, true)\n", "TestSP11_JCSScalars")
m("SP an array is sorted like an object", JCS, "\t\tfor i, v := range n.vals {\n\t\t\tif i > 0 {\n\t\t\t\tdst = append(dst, ',')\n\t\t\t}\n\t\t\tdst = appendJCS(dst, v)\n\t\t}\n", "\t\tfor i := len(n.vals) - 1; i >= 0; i-- {\n\t\t\tif i < len(n.vals)-1 {\n\t\t\t\tdst = append(dst, ',')\n\t\t\t}\n\t\t\tdst = appendJCS(dst, n.vals[i])\n\t\t}\n", "TestSP11_JCSSortsByUTF16")

# --- one mutant per rule kind: the kind loses a field it owns, or the
# requirement it carries is dropped. Killed by TestSP8_RuleKindFields/<kind>.
m("SP forbid_code loses exclude_paths", PARSE, '\t\tForbidCode:      {"pattern", "languages", "paths", "exclude_paths"},\n', '\t\tForbidCode:      {"pattern", "languages", "paths"},\n', "TestSP8_RuleKindFields/^forbid_code$")
m("SP forbid_code needs no pattern", WALK, "\t\t} else {\n\t\t\tp.failAt(\"pattern\", ReasonRequired, 0)\n\t\t}\n", "\t\t}\n", "TestSP8_RuleKindFields/^forbid_code_needs_a_pattern$")
m("SP forbid_code's pattern is not a regular expression", WALK, "\t\t\tr.Pattern = p.regex(pn)\n", "\t\t\tr.Pattern = p.str(pn, strRule{})\n", "TestSP5_RegexCapsCountInstructions")
m("SP forbid_command loses command", PARSE, '\t\tForbidCommand:   {"command"},\n', '\t\tForbidCommand:   {},\n', "TestSP8_RuleKindFields/^forbid_command$")
m("SP forbid_command needs no command", WALK, '\t\tr.Command = p.commandAt(f, "command", true)\n', '\t\tr.Command = p.commandAt(f, "command", false)\n', "TestSP8_RuleKindFields/^forbid_command_needs_a_command$")
m("SP protect_path loses modes", PARSE, '\t\tProtectPath:     {"paths", "modes"},\n', '\t\tProtectPath:     {"paths"},\n', "TestSP8_RuleKindFields/^protect_path$")
m("SP protect_path needs no paths", WALK, '\t\tr.Paths = p.strsAt(f, "paths", true, MaxListItems, pathRule)\n', '\t\tr.Paths = p.strsAt(f, "paths", false, MaxListItems, pathRule)\n', "TestSP8_RuleKindFields/^protect_path_needs_paths$")
m("SP protect_path takes four modes", WALK, '\t\tr.Modes = p.strsAt(f, "modes", false, len(Modes), in(Modes))\n', '\t\tr.Modes = p.strsAt(f, "modes", false, len(Modes)+1, in(Modes))\n', "TestSP8_RuleKindFields/^protect_path_modes_at_most_three$")
m("SP require_evidence loses before", PARSE, '\t\tRequireEvidence: {"at", "before", "evidence"},\n', '\t\tRequireEvidence: {"at", "evidence"},\n', "TestSP8_RuleKindFields/^require_evidence$")
m("SP require_evidence needs no evidence", WALK, '\t\tr.Evidence = p.strsAt(f, "evidence", true, MaxListItems, evidRule)\n', '\t\tr.Evidence = p.strsAt(f, "evidence", false, MaxListItems, evidRule)\n', "TestSP8_RuleKindFields/^require_evidence_needs_evidence$")
m("SP require_evidence needs neither at nor before", WALK, "r.At == nil && r.Before == nil", "r.At == nil && r.Before == nil && false", "TestSP8_RuleKindFields/^require_evidence_needs_at_or_before$")
m("SP require_evidence evidence is any word", PARSE, "\tevidRule  = strRule{cap: MaxWordBytes, vet: vetEvidence}\n", "\tevidRule  = strRule{cap: MaxWordBytes}\n", "TestSP8_RuleKindFields/^evidence_is_a_gate_id$")
m("SP order loses invalidated_by", PARSE, '\t\tOrder:           {"then", "after", "after_gate", "same_run_key", "invalidated_by"},\n', '\t\tOrder:           {"then", "after", "after_gate", "same_run_key"},\n', "TestSP8_RuleKindFields/^order$")
m("SP order needs no then", WALK, '\t\tr.Then = p.commandAt(f, "then", true)\n', '\t\tr.Then = p.commandAt(f, "then", false)\n', "TestSP8_RuleKindFields/^order_needs_then$")
m("SP order takes both after and after_gate", WALK, "\t\tcase r.After != nil && hasGate:\n", "\t\tcase false:\n", "TestSP8_RuleKindFields/^order_takes_one_of_after,_after_gate$")
m("SP order takes neither after nor after_gate", WALK, "\t\tcase r.After == nil && !hasGate:\n", "\t\tcase false:\n", "TestSP8_RuleKindFields/^order_needs_after_or_after_gate$")
m("SP a rule carries another kind's field", WALK, "\t\tif !slices.Contains(ruleCommon, k) && !slices.Contains(ruleOwn[r.Kind], k) {\n", "\t\tif false {\n", "TestSP8_RuleKindFields")
m("SP a rule's why is optional", WALK, '\tr.Why, _ = p.strAt(f, "why", true, whyRule)\n', '\tr.Why, _ = p.strAt(f, "why", false, whyRule)\n', "TestSP8_RuleKindFields")
m("SP a rule's hard_control is optional", WALK, '\tr.HardControl, _ = p.strAt(f, "hard_control", true, whyRule)\n', '\tr.HardControl, _ = p.strAt(f, "hard_control", false, whyRule)\n', "TestSP8_RuleKindFields")
m("SP an empty string is accepted", PARSE, "\tcase n.str == \"\":\n\t\tp.fail(ReasonEmpty, 0)\n", "", "TestSP8_RuleKindFields")
m("SP an empty list is accepted where one is required", PARSE, "\tcase len(n.vals) == 0 && nonEmpty:\n\t\tp.fail(ReasonEmpty, 0)\n", "", "TestSP8_RuleKindFields")

# --- command matches, paths, flags, SQL.
m("SP sql keywords go to any program", WALK, "!slices.Contains(SQLPrograms, c.Argv[0])", "false", "TestSP8_RuleKindFields")
m("SP sql keywords may be upper case only", VET, " || 'A' <= s[i] && s[i] <= 'Z'", "", "TestSP2_OverLimitRefused/^sql_items$")
m("SP sql keywords may hold any character", VET, "\t\tif !('a' <= s[i] && s[i] <= 'z' || 'A' <= s[i] && s[i] <= 'Z') {\n\t\t\treturn ReasonShape\n", "\t\tif false {\n\t\t\treturn ReasonShape\n", "TestSP8_RuleKindFields")
m("SP a flag need not start with a dash", VET, "len(s) < 2 || s[0] != '-' || vetWord(s) != \"\"", "len(s) < 2 || vetWord(s) != \"\"", "TestSP8_RuleKindFields")
m("SP a lone dash is a flag", VET, "len(s) < 2 || s[0] != '-' || vetWord(s) != \"\"", "len(s) < 1 || s[0] != '-' || vetWord(s) != \"\"", "TestSP8_RuleKindFields")
m("SP a flag may hold a space", VET, "len(s) < 2 || s[0] != '-' || vetWord(s) != \"\"", "len(s) < 2 || s[0] != '-'", "TestSP8_RuleKindFields/^a_flag_holds_no_space$")
m("SP flag_values keys need not be flags", WALK, "\t\tcase vetFlag(k) != \"\":\n", "\t\tcase false:\n", "TestSP8_RuleKindFields")
m("SP flag_values values may be empty lists", WALK, "\t\tout[k] = p.strList(&n.vals[i], MaxListItems, true, textRule)\n", "\t\tout[k] = p.strList(&n.vals[i], MaxListItems, false, textRule)\n", "TestSP8_RuleKindFields")
m("SP a rooted path is project-relative", VET, "\tif strings.HasPrefix(s, \"/\") || slices.Contains(strings.Split(s, \"/\"), \"..\") {\n", "\tif slices.Contains(strings.Split(s, \"/\"), \"..\") {\n", "TestSP8_PathsAreProjectRelative")
m("SP a path may climb out of the project", VET, "\tif strings.HasPrefix(s, \"/\") || slices.Contains(strings.Split(s, \"/\"), \"..\") {\n", "\tif strings.HasPrefix(s, \"/\") {\n", "TestSP8_PathsAreProjectRelative")
m("SP a path containing two dots is refused", VET, "slices.Contains(strings.Split(s, \"/\"), \"..\")", "strings.Contains(s, \"..\")", "TestSP8_PathsAreProjectRelative")

# --- the facts constraints (20.2.8, Facts; TestVER62).
m("SP an authorization class is accepted: authz", VET, '[]string{"authz", "forbidden", "403"}', '[]string{"forbidden", "403"}', "TestSP9_FactsConstraints")  # authz.token_expired is admitted by *_expired
m("SP an authorization class is accepted: forbidden", VET, '[]string{"authz", "forbidden", "403"}', '[]string{"authz", "403"}', "TestSP9_FactsConstraints")  # s3.forbidden, forbidden_missing
m("SP an authorization class is accepted: 403", VET, '[]string{"authz", "forbidden", "403"}', '[]string{"authz", "forbidden"}', "TestSP9_FactsConstraints")  # cred.403_missing is admitted by cred.*_missing
m("SP the deny list is never read", VET, '[]string{"authz", "forbidden", "403"}', '[]string{}', "TestSP9_FactsConstraints")
m("SP cred.registry is refused", VET, '\tcase "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":\n', '\tcase "cred.registryx", "cred.kube_context", "mcp.auth", "mcp.token_expired":\n', "TestSP9_FactsConstraints")
m("SP cred.kube_context is refused", VET, '\tcase "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":\n', '\tcase "cred.registry", "cred.kube_contextx", "mcp.auth", "mcp.token_expired":\n', "TestSP9_FactsConstraints")
m("SP mcp.auth is refused", VET, '\tcase "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":\n', '\tcase "cred.registry", "cred.kube_context", "mcp.authx", "mcp.token_expired":\n', "TestSP9_FactsConstraints")
m("SP cred.*_missing is refused", VET, 'wild("cred.", "_missing")', 'wild("cred.", "_missed")', "TestSP9_FactsConstraints")
m("SP *_profile_missing is refused", VET, 'wild("", "_profile_missing")', 'wild("", "_profile_missed")', "TestSP9_FactsConstraints")
m("SP *_expired is refused", VET, 'wild("", "_expired")', 'wild("", "_expire")', "TestSP9_FactsConstraints")
m("SP *_sso_needs_human is refused", VET, 'wild("", "_sso_needs_human")', 'wild("", "_sso_needs_humans")', "TestSP9_FactsConstraints")
m("SP a wildcard may match nothing", VET, "&& len(class) > len(prefix)+len(suffix)", "&& len(class) >= len(prefix)+len(suffix)", "TestSP9_FactsConstraints")  # cred._missing
m("SP any class is accepted", VET, "\treturn ReasonClassNotAllowed\n", "\treturn \"\"\n", "TestSP9_FactsConstraints")
m("SP a class may be 65 bytes", VET, "\tif len(s) > 64 {\n\t\treturn ReasonChars\n", "\tif len(s) > 65 {\n\t\treturn ReasonChars\n", "TestSP9_FactsConstraints")
m("SP a class may be upper case", VET, "\t\tif !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_')) {\n\t\t\treturn ReasonChars", "\t\tif false && c == 0 {\n\t\t\treturn ReasonChars", "TestSP9_FactsConstraints")
m("SP a class may start with a dot", VET, "'0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_')) {\n\t\t\treturn ReasonChars", "'0' <= c && c <= '9' || (c == '.' || c == '_')) {\n\t\t\treturn ReasonChars", "TestSP9_FactsConstraints")
m("SP classes are not checked", WALK, "\t\tif r := classReason(out[i]); r != \"\" {\n", "\t\tif r := classReason(out[i]); r != \"\" && false {\n", "TestSP9_FactsConstraints")
m("SP a fact may carry no classes", WALK, "\tout := p.strList(n, MaxListItems, true, classRule)\n", "\tout := p.strList(n, MaxListItems, false, classRule)\n", "TestSP9_FactsConstraints")
m("SP a fact needs no classes key", WALK, "\t} else {\n\t\tp.failAt(\"classes\", ReasonRequired, 0)\n\t}\n", "\t}\n", "TestSP9_FactsConstraints")
m("SP a fact needs no programs", WALK, '\tft.Programs = p.strsAt(f, "programs", true, MaxListItems, idRule)\n', '\tft.Programs = p.strsAt(f, "programs", false, MaxListItems, idRule)\n', "TestSP9_FactsConstraints")
m("SP a fact's program is any word", WALK, '\tft.Programs = p.strsAt(f, "programs", true, MaxListItems, idRule)\n', '\tft.Programs = p.strsAt(f, "programs", true, MaxListItems, wordRule)\n', "TestSP9_FactsConstraints")
m("SP a fact needs no value", WALK, '\tft.Value, _ = p.strAt(f, "value", true, valueRule)\n', '\tft.Value, _ = p.strAt(f, "value", false, valueRule)\n', "TestSP9_FactsConstraints")
m("SP a fact needs no mechanism", WALK, 'if mech, _ := p.strAt(f, "mechanism", true, strRule{cap: MaxFactValueBytes}); p.err == nil {', 'if mech, _ := p.strAt(f, "mechanism", false, strRule{cap: MaxFactValueBytes}); p.err == nil && mech != "" {', "TestSP9_FactsConstraints")
m("SP a fact value may hold a space", VET, "\t\tif !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte(\"._:/@+=-\", c) >= 0) {\n\t\t\treturn ReasonChars", "\t\tif !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte(\"._:/@+=- \", c) >= 0) {\n\t\t\treturn ReasonChars", "TestSP9_FactsConstraints")
m("SP a fact value may hold a comma", VET, "strings.IndexByte(\"._:/@+=-\", c) >= 0) {\n\t\t\treturn ReasonChars", "strings.IndexByte(\"._:/@+=-,\", c) >= 0) {\n\t\t\treturn ReasonChars", "TestSP9_FactsConstraints")
m("SP a fact value may not hold a colon", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._/@+=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold a slash", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._:@+=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold an at sign", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._:/+=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold a plus", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._:/@=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold an equals sign", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._:/@+-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold a dot", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"_:/@+=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold an underscore", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\".:/@+=-\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may not hold a hyphen", VET, "strings.IndexByte(\"._:/@+=-\", c)", "strings.IndexByte(\"._:/@+=\", c)", "TestSP9_FactsConstraints")
m("SP a fact value may be a URL", VET, "\tif strings.Contains(s, \"://\") {\n", "\tif strings.Contains(s, \"://\") && false {\n", "TestSP9_FactsConstraints")
m("SP a fact's mechanism need not match its kind", VET, "\tcase FactKinds[i] != kind:\n", "\tcase false:\n", "TestSP2_ClosedEnumsAndIDs")
m("SP a mechanism may be any word", VET, "\tcase i < 0:\n\t\treturn ReasonEnum\n", "\tcase i < 0:\n\t\treturn \"\"\n", "TestSP9_FactsConstraints")
m("SP a mechanism may hold a space", VET, "\tcase strings.Contains(mech, \" \"):\n", "\tcase false:\n", "TestSP9_MechanismSpacesConflict")
m("SP a fact target needs no id", WALK, '\tt.ID, _ = p.strAt(f, "id", true, valueRule)\n', '\tt.ID, _ = p.strAt(f, "id", false, valueRule)\n', "TestSP9_FactsConstraints")
m("SP a fact target needs no kind", WALK, '\tt.Kind, _ = p.strAt(f, "kind", true, in(TargetKinds))\n', '\tt.Kind, _ = p.strAt(f, "kind", false, in(TargetKinds))\n', "TestSP9_FactsConstraints")
