# VER-37: spec.json is read strictly (internal/spec). Each break removes one
# rule of the schema or moves one cap by a step; the kill row is the row of
# TestVER37_SpecStrictSchema (or the named test) that names that rule.
T = "TestVER37_|TestVER37b_|TestVER37d_|TestVER37e_"
P = "internal/spec/parse.go"
R = "internal/spec/regex.go"
TREE = "internal/spec/tree.go"
S = "internal/spec/spec.go"

# Version, shape, unknown keys.
m("VER-37 a version other than 1 is accepted", P, "\tcase v.num != 1:\n", "\tcase false:\n", T)
m("VER-37 a version that is not an integer value is not refused as a type", P, "\tcase v.kind != kInt:\n", "\tcase false:\n", T)
m("VER-37 an unknown key is accepted", P, "if f != nil && !slices.Contains(allowed, k) {", "if false {", T)
m("VER-37 a missing required member is accepted (gate argv)", P,
  'g.Argv = field(r, f, "argv", true, r.words)', 'g.Argv = field(r, f, "argv", false, r.words)', T)
m("VER-37 a missing required member is accepted (rule why)", P,
  'x.Why = field(r, f, "why", true, r.why)', 'x.Why = field(r, f, "why", false, r.why)', T)
m("VER-37 a missing required member is accepted (fact programs)", P,
  'x.Programs = field(r, f, "programs", true, r.words)', 'x.Programs = field(r, f, "programs", false, r.words)', T)
m("VER-37 an empty list is accepted", P, "\tcase len(n.vals) == 0:\n", "\tcase false:\n", T)
m("VER-37 an empty map is accepted", P, "} else if len(n.keys) == 0 {", "} else if false {", T)
m("VER-37 the first error in a file is not the first in walk order (facts before rules)", P,
  '\teach(r, "rules", rules, func(n *node, seen map[string]struct{}) { f.Rules = append(f.Rules, r.rule(n, seen)) })\n'
  '\teach(r, "facts", facts, func(n *node, seen map[string]struct{}) { f.Facts = append(f.Facts, r.fact(n, seen)) })\n',
  '\teach(r, "facts", facts, func(n *node, seen map[string]struct{}) { f.Facts = append(f.Facts, r.fact(n, seen)) })\n'
  '\teach(r, "rules", rules, func(n *node, seen map[string]struct{}) { f.Rules = append(f.Rules, r.rule(n, seen)) })\n', T)

# Counts.
m("VER-37 gates may exceed their cap by one", P, "case len(gates) > MaxGates:", "case len(gates) > MaxGates+1:", T)
m("VER-37 ci_commands do not count toward the gate cap", P, "case len(gates)+len(ci) > MaxGates:", "case len(ci) > MaxGates:", T)
m("VER-37 rules are capped one early", P, "case len(rules) > MaxRules:", "case len(rules) >= MaxRules:", T)
m("VER-37 facts may exceed their cap by one", P, "case len(facts) > MaxFacts:", "case len(facts) > MaxFacts+1:", T)
m("VER-37 a list is read before its count is checked", P,
  "\tcase r.err != nil:\n\tcase len(gates) > MaxGates:\n", "\tcase r.err != nil:\n\tcase false:\n", T)

# Ids.
m("VER-37 an id may be one byte too long", P, "if s == \"\" || len(s) > MaxIDBytes {", "if s == \"\" || len(s) > MaxIDBytes+1 {", T)
m("VER-37 an empty id is accepted", P, "if s == \"\" || len(s) > MaxIDBytes {", "if len(s) > MaxIDBytes {", T)
m("VER-37 an id may start with a dot, dash or underscore", P,
  "|| i > 0 && (c == '.' || c == '_' || c == '-')) {", "|| (c == '.' || c == '_' || c == '-')) {", T)
m("VER-37 an id may hold an upper-case letter", P,
  "if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 &&", "if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || i > 0 &&", T)
m("VER-37 an id may hold a space", P,
  "|| i > 0 && (c == '.' || c == '_' || c == '-')) {", "|| i > 0 && (c == '.' || c == '_' || c == '-' || c == ' ')) {", T)
m("VER-37 a repeated id is accepted", P, "\t\t\tseen[n.str] = struct{}{}\n", "\t\t\t_ = n.str\n", T)
m("VER-37 after_gate is read as plain text", P,
  'x.AfterGate = field(r, f, "after_gate", false, r.id(nil))', 'x.AfterGate = field(r, f, "after_gate", false, r.plain)', T)
m("VER-37 an evidence kind is not recognised", P, 'strings.CutPrefix(n.str, "kind:")', 'strings.CutPrefix(n.str, "kind")', T)
m("VER-37 an evidence kind outside the gate kinds is accepted", P,
  "if !slices.Contains(gateKinds, k) {", "if k == \"\" {", T)

# Rune caps and text.
m("VER-37 why is capped in bytes, not runes", P, "utf8.RuneCountInString(s) > maxRunes", "len(s) > maxRunes", T)
IMPORTS["VER-37 why is capped in bytes, not runes"] = (P, '\t"unicode/utf8"\n', "")
m("VER-37 a rune cap of 0 is not unlimited", P, "case maxRunes > 0 && utf8", "case utf8", T)
m("VER-37 why is uncapped", P, "return r.text(n, MaxWhyRunes) }", "return r.text(n, 0) }", T)
m("VER-37 alternative is capped like why", P, "return r.text(n, MaxAlternativeRunes) }", "return r.text(n, MaxWhyRunes) }", T)
m("VER-37 hard_control is uncapped", P,
  'x.HardControl = field(r, f, "hard_control", true, r.why)', 'x.HardControl = field(r, f, "hard_control", true, r.plain)', T)
m("VER-37 a fact's why is uncapped", P,
  'x.Why = field(r, f, "why", false, r.why)', 'x.Why = field(r, f, "why", false, r.plain)', T)
m("VER-37 an empty string is accepted", P, '\tcase s == "":\n', "\tcase false:\n", T)
m("VER-37 a control character is accepted", P, "case slices.ContainsFunc([]rune(s), unicode.IsControl):", "case false:", T)

# Defaults.
m("VER-37 the default match is exact", P, 'g.Match = "prefix"', 'g.Match = "exact"', T)
m("VER-37 the default match is never applied", P, 'if g.Match == "" {', 'if g.Match == "x" {', T)
m("VER-37 the default required is false", P, "yes := true", "yes := false", T)
m("VER-37 the default required is never applied", P, "if g.Required == nil {", "if g.Required != nil {", T)
m("VER-37 an explicit required is overwritten", P, "if g.Required == nil {", "if true {", T)
m("VER-37 the default modes are shared between rules", P, "x.Modes = slices.Clone(pathModes)", "x.Modes = pathModes", T)
m("VER-37 modes default on every rule kind", P, 'if x.Kind == "protect_path" && x.Modes == nil {', "if x.Modes == nil {", T)
m("VER-37 modes default only when the kind is not protect_path", P, 'if x.Kind == "protect_path" && x.Modes == nil {', 'if x.Kind != "protect_path" && x.Modes == nil {', T)
m("VER-37 explicit modes are overwritten", P, 'if x.Kind == "protect_path" && x.Modes == nil {', 'if x.Kind == "protect_path" {', T)

# Closed lists: one member dropped, or one added.
m("VER-37 gate kind scan is refused", P, '"typecheck", "scan", "custom"}', '"typecheck", "custom"}', T)
m("VER-37 gate kind deploy is accepted", P, '"typecheck", "scan", "custom"}', '"typecheck", "scan", "custom", "deploy"}', T)
m("VER-37 match glob is accepted", P, 'matchModes  = []string{"prefix", "exact"}', 'matchModes  = []string{"prefix", "exact", "glob"}', T)
m("VER-37 match exact is refused", P, 'matchModes  = []string{"prefix", "exact"}', 'matchModes  = []string{"prefix"}', T)
m("VER-37 rule kind order is refused", P, '"protect_path", "order"}', '"protect_path"}', T)
m("VER-37 rule kind forbid_file is accepted", P, '"protect_path", "order"}', '"protect_path", "order", "forbid_file"}', T)
m("VER-37 mode chmod is accepted", P, '[]string{"modify", "delete", "create"}', '[]string{"modify", "delete", "create", "chmod"}', T)
m("VER-37 mode create is refused", P, '[]string{"modify", "delete", "create"}', '[]string{"modify", "delete"}', T)
m("VER-37 at merge is accepted", P, '"done", "subtask"}', '"done", "subtask", "merge"}', T)
m("VER-37 at subtask is refused", P, '"done", "subtask"}', '"done"}', T)
m("VER-37 fact kind aws_secret is accepted", P, '"env_var_name", "sign_in_step"}', '"env_var_name", "sign_in_step", "aws_secret"}', T)
m("VER-37 fact kind sign_in_step is refused", P, '"env_var_name", "sign_in_step"}', '"env_var_name"}', T)
m("VER-37 mechanism token is accepted", P, '"env var name", "sign-in step"}', '"env var name", "sign-in step", "token"}', T)
m("VER-37 mechanism sign-in step is refused", P, '"env var name", "sign-in step"}', '"env var name"}', T)
m("VER-37 target kind aws_user is accepted", P, '"gcp_project", "azure_subscription"}', '"gcp_project", "azure_subscription", "aws_user"}', T)
m("VER-37 target kind azure_subscription is refused", P, '"gcp_project", "azure_subscription"}', '"gcp_project"}', T)
m("VER-37 an enum is matched in any case", P, "} else if !slices.Contains(set, n.str) {", "} else if !slices.Contains(set, strings.ToLower(n.str)) {", T)
m("VER-37 an enum member of the wrong type is accepted as its text", P,
  "\t\tif n.kind != kString {\n\t\t\tr.fail(ReasonType, 0)\n\t\t} else if !slices.Contains(set,", "\t\tif false {\n\t\t} else if !slices.Contains(set,", T)

# Regular expressions are read where the archive puts them.
m("VER-37 a rule pattern is not checked as a regex", P, 'x.Pattern = field(r, f, "pattern", false, r.regex)', 'x.Pattern = field(r, f, "pattern", false, r.plain)', T)
m("VER-37 success_line is not checked as a regex", P, 'g.SuccessLine = field(r, f, "success_line", false, r.regexes)', 'g.SuccessLine = field(r, f, "success_line", false, r.words)', T)
m("VER-37 failure_line is not checked as a regex", P, 'g.FailureLine = field(r, f, "failure_line", false, r.regexes)', 'g.FailureLine = field(r, f, "failure_line", false, r.words)', T)
m("VER-37 empty_run_line is not checked as a regex", P, 'g.EmptyRunLine = field(r, f, "empty_run_line", false, r.regexes)', 'g.EmptyRunLine = field(r, f, "empty_run_line", false, r.words)', T)
m("VER-37 the spec-wide instruction total stops at the limit instead of passing it", P, "r.insts > MaxSpecInstructions", "r.insts >= MaxSpecInstructions", T)
m("VER-37 the spec-wide instruction total forgets earlier patterns", P, "if r.insts += insts;", "if r.insts = insts;", T)
m("VER-37 the spec-wide instruction total is not enforced", P, "r.insts > MaxSpecInstructions", "r.insts > MaxSpecInstructions*100", T)
m("VER-37 the file cap is one over", TREE, "if len(data) > MaxBytes {", "if len(data) > MaxBytes+1 {", T)
m("VER-37 the file cap is one under", TREE, "if len(data) > MaxBytes {", "if len(data) >= MaxBytes {", T)

# Regex bounds (regex.go).
m("VER-37 a pattern of exactly 512 bytes is refused", R, "if len(src) > MaxPatternBytes {", "if len(src) >= MaxPatternBytes {", T)
m("VER-37 a pattern of 513 bytes is accepted", R, "if len(src) > MaxPatternBytes {", "if len(src) > MaxPatternBytes+1 {", T)
m("VER-37 Go's own repeat ceiling is reported as a syntax error", R, "se.Code == syntax.ErrInvalidRepeatSize", "se.Code == syntax.ErrMissingParen", T)
m("VER-37 a pattern is parsed with POSIX, not Perl, syntax", R, "syntax.Parse(src, syntax.Perl)", "syntax.Parse(src, syntax.POSIX)", T)
m("VER-37 a pattern of exactly 2000 instructions is refused", R, "if len(prog.Inst) > MaxPatternInstructions {", "if len(prog.Inst) >= MaxPatternInstructions {", T)
m("VER-37 a pattern of 2001 instructions is accepted", R, "if len(prog.Inst) > MaxPatternInstructions {", "if len(prog.Inst) > MaxPatternInstructions+1 {", T)
m("VER-37 a repeat of exactly 100 is refused", R, "if re.Min > MaxRepeat || re.Max > MaxRepeat {", "if re.Min >= MaxRepeat || re.Max >= MaxRepeat {", T)
m("VER-37 the repeat maximum is not checked", R, "if re.Min > MaxRepeat || re.Max > MaxRepeat {", "if re.Min > MaxRepeat {", T)
m("VER-37 the repeat minimum is not checked", R, "if re.Min > MaxRepeat || re.Max > MaxRepeat {", "if re.Max > MaxRepeat {", T)
m("VER-37 a nested counted repeat is accepted", R, "\t\tif inside {\n", "\t\tif false {\n", T)
m("VER-37 the walk forgets it is inside a counted repeat", R, "\t\tinside = true\n", "\t\tinside = false\n", T)
m("VER-37 the repeat walk does not descend", R, "for _, sub := range re.Sub {", "for _, sub := range re.Sub[:0] {", T)

# Lexical rules (tree.go).
m("VER-37 nesting may reach one level too deep", TREE, "if depth+1 > MaxDepth {", "if depth+1 > MaxDepth+1 {", T)
m("VER-37 nesting stops one level early", TREE, "if depth+1 > MaxDepth {", "if depth+1 >= MaxDepth {", T)
m("VER-37 a duplicate key is accepted", TREE, "\t\tseen[key] = struct{}{}\n", "\t\t_ = key\n", T)
m("VER-37 a non-integer number is accepted", TREE, "\tcase err != nil:\n\t\tb.fail(ReasonNotInteger, 0)", "\tcase false:\n\t\tb.fail(ReasonNotInteger, 0)", T)
m("VER-37 an integer above 2^53-1 is accepted", TREE, ", v > maxSafeInt, v < -maxSafeInt:", ", v > maxSafeInt+1, v < -maxSafeInt:", T)
m("VER-37 an integer below -(2^53-1) is accepted", TREE, ", v > maxSafeInt, v < -maxSafeInt:", ", v > maxSafeInt:", T)
m("VER-37 a value after the document is accepted", TREE, "err != io.EOF {", "err == nil {", T)
IMPORTS["VER-37 a value after the document is accepted"] = (TREE, '\t"io"\n', "")
m("VER-37 a lexical fault is reported on any token", TREE, "if b.bad >= 0 && int(b.dec.InputOffset()) > b.bad {", "if true && int(b.dec.InputOffset()) > b.bad {", T)
m("VER-37 a lexical fault is reported at the token before it", TREE, "int(b.dec.InputOffset()) > b.bad {", "int(b.dec.InputOffset()) >= b.bad {", T)
m("VER-37 a literal U+FFFD is refused as invalid UTF-8", TREE, "if r == utf8.RuneError && size == 1 {", "if r == utf8.RuneError {", T)
m("VER-37 invalid UTF-8 is accepted", TREE, "if r == utf8.RuneError && size == 1 {", "if false {", T)
IMPORTS["VER-37 invalid UTF-8 is accepted"] = (TREE, "\t\t\tr, size := utf8.DecodeRune(data[i:])\n\t\t\tif false {", "\t\t\tr, size := utf8.DecodeRune(data[i:])\n\t\t\t_ = r\n\t\t\tif false {")
m("VER-37 a lone low surrogate is accepted at its first unit", TREE, "case unit >= 0xDC00 && unit <= 0xDFFF:", "case unit > 0xDC00 && unit <= 0xDFFF:", T)
m("VER-37 a lone low surrogate is accepted at its last unit", TREE, "case unit >= 0xDC00 && unit <= 0xDFFF:", "case unit >= 0xDC00 && unit < 0xDFFF:", T)
m("VER-37 a lone high surrogate is accepted at its first unit", TREE, "case unit >= 0xD800 && unit <= 0xDBFF:", "case unit > 0xD800 && unit <= 0xDBFF:", T)
m("VER-37 a lone high surrogate is accepted at its last unit", TREE, "case unit >= 0xD800 && unit <= 0xDBFF:", "case unit >= 0xD800 && unit < 0xDBFF:", T)
m("VER-37 a high surrogate may be followed by a code above the low range", TREE, "if !ok || low < 0xDC00 || low > 0xDFFF {", "if !ok || low < 0xDC00 {", T)
m("VER-37 a high surrogate may be followed by a code below the low range", TREE, "if !ok || low < 0xDC00 || low > 0xDFFF {", "if !ok || low > 0xDFFF {", T)
m("VER-37 a high surrogate may be followed by any escape", TREE, "if !ok || low < 0xDC00 || low > 0xDFFF {", "if !ok || low < 0 {", T)
m("VER-37 a valid pair is read as two escapes", TREE, "\t\t\t\ti += 12\n", "\t\t\t\ti += 6\n", T)
m("VER-37 an escaped backslash is read as one character", TREE, "\t\tcase c == '\\\\':\n\t\t\ti += 2\n", "\t\tcase c == '\\\\':\n\t\t\ti++\n", T)
m("VER-37 strings are never entered", TREE, "\t\t\tinString = c == '\"'\n", "\t\t\tinString = false\n", T)
# Not registered, equivalent: a \u escape whose four characters are not hex
# digits (or that is cut off by the end of the file) is refused by the JSON
# decoder as a syntax error when it reads that token, before the offset
# lexicalFault returns is ever compared; so returning a surrogate fault there
# instead of stepping over the two bytes changes no result Parse can give.
# Register it again if lexicalFault's result is used before the decoder reads.
# The same holds for leaving the string state set after a closing quote:
# a backslash outside a string is a syntax error too.

# Pointers.
m("VER-37 a key of 49 bytes is echoed in a pointer", TREE, "if len(s) > MaxIDBytes {\n\t\treturn false", "if len(s) > MaxIDBytes+1 {\n\t\treturn false", T)
m("VER-37 a pointer shows any key", TREE, "if safeSegment(s) {", "if true {", T)
m("VER-37 a pointer hides a dot", TREE, "c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "c == '_' || c == '-') {\n\t\t\treturn false", T)
m("VER-37 a pointer hides an underscore", TREE, "c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "c == '.' || c == '-') {\n\t\t\treturn false", T)
m("VER-37 a pointer hides a dash", TREE, "c == '.' || c == '_' || c == '-') {\n\t\t\treturn false", "c == '.' || c == '_') {\n\t\t\treturn false", T)
m("VER-37 a pointer hides an upper-case letter", TREE, "|| 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.'", "|| '0' <= c && c <= '9' || c == '.'", T)
m("VER-37 a pointer hides a digit", TREE, "|| 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.'", "|| 'A' <= c && c <= 'Z' || c == '.'", T)
m("VER-37 a hidden segment is written with its own text", TREE, "out = append(out, '?')", "out = append(out, s[:1]...)", T)

# Returned values.
m("VER-37 a refused spec still returns its File", P, "return nil, digest, r.err", "return f, digest, r.err", T)

# Caps as constants.
m("VER-37 MaxBytes is one KiB short", S, "MaxBytes               = 256 << 10", "MaxBytes               = 255 << 10", T)
m("VER-37 MaxGates is 33", S, "MaxGates               = 32 // including ci_commands", "MaxGates               = 33 // including ci_commands", T)
m("VER-37 MaxRules is 65", S, "MaxRules               = 64\n", "MaxRules               = 65\n", T)
m("VER-37 MaxFacts is 65", S, "MaxFacts               = 64\n", "MaxFacts               = 65\n", T)
m("VER-37 MaxPatternBytes is 513", S, "MaxPatternBytes        = 512", "MaxPatternBytes        = 513", T)
m("VER-37 MaxPatternInstructions is 2001", S, "MaxPatternInstructions = 2000", "MaxPatternInstructions = 2001", T)
m("VER-37 MaxSpecInstructions is 20001", S, "MaxSpecInstructions    = 20000", "MaxSpecInstructions    = 20001", T)
m("VER-37 MaxRepeat is 101", S, "MaxRepeat              = 100", "MaxRepeat              = 101", T)
m("VER-37 MaxWhyRunes is 299", S, "MaxWhyRunes = 300", "MaxWhyRunes = 299", T)
m("VER-37 MaxAlternativeRunes is 161", S, "MaxAlternativeRunes = 160", "MaxAlternativeRunes = 161", T)
m("VER-37 MaxIDBytes is 49", S, "MaxIDBytes = 48", "MaxIDBytes = 49", T)
m("VER-37 MaxDepth is 33", S, "MaxDepth = 32", "MaxDepth = 33", T)

# Facts (archive 20.2.8, Facts): identity kinds fire on authentication classes only.
m("VER-37 an identity fact's classes are not checked", P, "\tif slices.Contains(identityFactKinds, x.Kind) {\n", "\tif false {\n", T)
m("VER-37 every fact kind is restricted to authentication classes", P, "\tif slices.Contains(identityFactKinds, x.Kind) {\n", "\tif true {\n", T)
m("VER-37 aws_region is not an identity kind", P, 'identityFactKinds = []string{"aws_profile", "aws_region",', 'identityFactKinds = []string{"aws_profile",', T)
m("VER-37 kube_namespace is not an identity kind", P, '"kube_namespace", "package_index_host"}', '"package_index_host"}', T)
m("VER-37 package_index_host is not an identity kind", P, '"kube_namespace", "package_index_host"}', '"kube_namespace"}', T)
m("VER-37 kube_context is not an identity kind", P, '"kube_context", "kube_namespace", "package_index_host"}\n\n// authnClasses', '"kube_namespace", "package_index_host"}\n\n// authnClasses', T)
m("VER-37 authz classes are not refused outright", P, 'strings.HasPrefix(c, "authz.") || ', "", T)
m("VER-37 forbidden classes are not refused outright", P, 'strings.Contains(c, "forbidden") || ', "", T)
m("VER-37 403 classes are not refused outright", P, ' || strings.Contains(c, "403") {', " {", T)
m("VER-37 cred.registry is refused", P, 'case "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":', 'case "cred.kube_context", "mcp.auth", "mcp.token_expired":', T)
m("VER-37 cred.kube_context is refused", P, 'case "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":', 'case "cred.registry", "mcp.auth", "mcp.token_expired":', T)
m("VER-37 mcp.auth is refused", P, 'case "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":', 'case "cred.registry", "cred.kube_context", "mcp.token_expired":', T)
# Not registered, equivalent: mcp.token_expired also ends "_expired", so the
# explicit case in authnClass is redundant with the suffix test; it is kept
# because the spec lists it by name. Register it if the suffix test changes.
m("VER-37 a cred. class needs no _missing suffix", P, 'strings.HasPrefix(c, "cred.") && strings.HasSuffix(c, "_missing")', 'strings.HasPrefix(c, "cred.")', T)
m("VER-37 a _missing class needs no cred. prefix", P, 'strings.HasPrefix(c, "cred.") && strings.HasSuffix(c, "_missing")', 'strings.HasSuffix(c, "_missing")', T)
m("VER-37 _profile_missing is refused", P, 'strings.HasSuffix(c, "_profile_missing") || ', "", T)
m("VER-37 _expired is refused", P, 'strings.HasSuffix(c, "_expired") ||', "", T)
m("VER-37 _sso_needs_human is refused", P, ' ||\n\t\tstrings.HasSuffix(c, "_sso_needs_human")', "", T)
m("VER-37 only the first class is checked", P, "\tfor i, c := range classes {", "\tfor i, c := range classes[:min(1, len(classes))] {", T)
m("VER-37 a fact value may hold a space", P, "strings.IndexFunc(s, unicode.IsSpace) >= 0", "strings.Contains(s, \"\\t\")", T)
m("VER-37 a fact value is not read as an identifier", P, 'field(r, f, "value", true, r.identifier)', 'field(r, f, "value", true, r.plain)', T)
