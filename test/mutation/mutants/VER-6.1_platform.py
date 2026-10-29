# VER-6.1: platform impact (internal/platform), PF-PL8 and D132. One break per
# promise: the deadline and what a timed-out call returns, a package the log
# does not hold, a retraction, the header, the cap, the record grammar, the
# row's kind, family and evidence, a family's members, and Go's package rule
# for a written path. The kill row is the named test's.
P = "internal/platform/platform.go"
D = "internal/platform/decode.go"
R = "internal/platform/resolve.go"

# The deadline: unknown, at the deadline, never partial.
m("VER-6.1 Deadline ignored", P,
  '\tctx, cancel := context.WithTimeout(context.Background(), Deadline)\n',
  '\tctx, cancel := context.WithCancel(context.Background())\n', "TestPL8c_")
m("VER-6.1 a timed-out call waits for its worker", P,
  '\tcase <-ctx.Done():\n\t\treturn nil, ctx.Err()\n',
  '\tcase <-ctx.Done():\n\t\tr := <-done\n\t\treturn r.fams, r.err\n', "TestPL8c_")
m("VER-6.1 a cancelled decode returns the facts read so far", D,
  '\t\t\tif err := ctx.Err(); err != nil {\n\t\t\t\treturn nil, err\n',
  '\t\t\tif err := ctx.Err(); err != nil {\n\t\t\t\treturn live, nil\n', "TestPL8c3_")
m("VER-6.1 the decoder never looks at its context", D,
  '\t\tif n%checkEvery == 0 {\n', '\t\tif false {\n', "TestPL8c3_")

# Families.
m("VER-6.1 a package the log does not hold gets a family", P,
  '\t\tif _, ok := bySubject["pkg:"+pkg]; !ok {\n', '\t\tif _, ok := bySubject["pkg:"+pkg]; !ok && false {\n', "TestPL8b_")
m("VER-6.1 a retraction leaves its fact live", D,
  '\t\t} else {\n\t\t\tdelete(live, f)\n\t\t}\n', '\t\t}\n', "TestPL8g_")
m("VER-6.1 a type's methods are not in its package's family", P,
  '\t\tfor _, m := range bySubject[d.object] {\n\t\t\tif m.predicate == predDeclares {\n\t\t\t\tmembers = append(members, m.object)\n\t\t\t}\n\t\t}\n',
  '', "TestPL8a_")

# The format: header, cap, record grammar.
m("VER-6.1 any header is read as v0", D, '\tif string(header) != LogHeader {\n', '\tif string(header) != LogHeader && false {\n', "TestPL8e_")
m("VER-6.1 no byte cap", P, '\tif info.Size() > limit {\n', '\tif info.Size() > limit && false {\n', "TestPL8f_")
m("VER-6.1 a log at the cap is refused", P, '\tif info.Size() > limit {\n', '\tif info.Size() >= limit {\n', "TestPL8f_")
m("VER-6.1 an empty field is accepted", D,
  '\t\tif s == "" || len(s) > maxFieldBytes ||', '\t\tif len(s) > maxFieldBytes ||', "TestPL8e2_")
m("VER-6.1 an unknown tier is accepted", D, '\tif tierRank(c[4]) == 0 {\n', '\tif false {\n', "TestPL8e2_")
m("VER-6.1 a torn last line is accepted", D,
  '\tcase err == io.EOF:\n\t\treturn nil, errors.New("the log lacks a final newline")\n',
  '\tcase err == io.EOF:\n\t\treturn line, nil\n', "TestPL8e2_")

# The row.
m("VER-6.1 the row's kind is not platform_impact", P,
  'Scope: verify.ScopeSession, Kind: verify.PlatformImpact, Source:', 'Scope: verify.ScopeSession, Kind: verify.SpecRule, Source:', "TestPL8d_")
m("VER-6.1 the row's evidence is null", P, '\t\tEvidence: []int64{}}\n', '\t\tEvidence: nil}\n', "TestPL8d_")
m("VER-6.1 the row's family leaves out the Hash", P,
  'platformRow(f.Package+"@"+f.Hash, verify.ObligationOpen)', 'platformRow(f.Package, verify.ObligationOpen)', "TestPL8d_")

# Go's package rule (D132).
m("VER-6.1 the nearest go.mod is skipped for the root's", R,
  '\t\tif m := moduleAt(root, d, mods); m.found {\n', '\t\tif m := moduleAt(root, d, mods); m.found && d == "." {\n', "TestPL8i_")
m("VER-6.1 a go.mod without a module line is passed over", R,
  '\t\tif m := moduleAt(root, d, mods); m.found {\n', '\t\tif m := moduleAt(root, d, mods); m.found && m.path != "" {\n', "TestPL8i_")
m("VER-6.1 testdata holds a package", R,
  '\t\tif e == "testdata" || e != "."', '\t\tif e != "."', "TestPL8i_")
m("VER-6.1 a dot directory holds a package", R,
  '(strings.HasPrefix(e, ".") || strings.HasPrefix(e, "_"))', 'strings.HasPrefix(e, "_")', "TestPL8i_")
m("VER-6.1 a leading comment hides the module line", R,
  '\t\tif !ok || rest == "" || (rest[0] != \' \' && rest[0] != \'\\t\') {\n\t\t\tcontinue\n\t\t}\n',
  '\t\tif !ok || rest == "" || (rest[0] != \' \' && rest[0] != \'\\t\') {\n\t\t\treturn goMod{found: true}\n\t\t}\n', "TestPL8i_")
m("VER-6.1 a trailing comment stays in the module path", R,
  '\t\tif i := strings.Index(line, "//"); i >= 0 {\n\t\t\tline = line[:i]\n\t\t}\n', '', "TestPL8i_")
m("VER-6.1 a quoted module path keeps its quotes", R,
  '\t\tif u, err := strconv.Unquote(mod); err == nil {\n', '\t\tif u, err := strconv.Unquote(mod); err == nil && false {\n', "TestPL8i_")
