# QUAL-MEMBER1: Type.member names the unit that declares the member, not the
# first unit that declares the type (NN17 Tally.Kept, NN19 File.Prompts). One
# row per branch of the rule in resolveQualified: the declarer that holds the
# member wins, a lone declarer keeps its unit when the member is not indexed,
# several declarers with none holding the member name no unit and say why,
# and a unit declaring the type twice is one declarer.
N = "internal/graph/engine/names.go"
m("QUAL-MEMBER1 the first declarer of the type is the unit, member or not", N,
  "if _, held := c.symIn(r, u); held {", "if true {", "TestMember_ATypeInTwoUnitsNamesTheOneThatHoldsTheMember")
m("QUAL-MEMBER1 a lone declarer loses its unit when the member is not indexed", N,
  "case len(declarers) == 1:", "case len(declarers) == 0:", "TestMember_ATypeInOneUnitKeepsItsUnitWhenTheMemberIsNotIndexed")
m("QUAL-MEMBER1 several declarers with no holder still name the first", N,
  "\t\t\tnw.Why = r + \" is declared in none of the \" + strconv.Itoa(len(declarers)) + \" units that declare \" + l\n\t\t\treturn nw, true\n",
  "\t\t\tunit, ok = declarers[0], true\n",
  "TestMember_ATypeInTwoUnitsWithoutTheMemberNamesNoUnit|TestMember_APositiveTwinOfTheUndeclaredMember")
m("QUAL-MEMBER1 the word with no holder is not a qualified word at all", N,
  "case len(declarers) > 1:", "case len(declarers) > 2:", "TestMember_ATypeInTwoUnitsWithoutTheMemberNamesNoUnit")
m("QUAL-MEMBER1 the reason does not say none of the units holds the member", N,
  "\" is declared in none of the \"", "\" is declared in the \"", "TestMember_ATypeInTwoUnitsWithoutTheMemberNamesNoUnit")
m("QUAL-MEMBER1 a unit that declares the type twice counts as two declarers", N,
  "!slices.Contains(declarers, u)", "true", "TestMember_ATypeInOneUnitKeepsItsUnitWhenTheMemberIsNotIndexed")
m("QUAL-MEMBER1 a unit-segment qualifier reads the type first", N,
  "\tunit, ok := c.unitBySegment(l)\n\tif !ok {\n\t\tvar declarers", "\tunit, ok := \"\", false\n\tif !ok {\n\t\tvar declarers",
  "TestMember_AUnitSegmentQualifierIsUnchanged")
