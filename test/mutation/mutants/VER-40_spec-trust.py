# VER-40: `rashomon spec trust` binds a repository spec by its digest (card
# SPEC-TRUST1, PRD PF-SP2). One break per promise the binding check keeps: an
# edit (a new digest) still reads as trusted, the same bytes at another path
# of the project read as trusted, and a bindings file over its cap is read.
# Each term is weakened, never deleted. The kill rows are discovery's own
# test through the steer step, and the verb's edit test.
T = "internal/spec/trust.go"
HOLDS = "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath && x.SpecDigest == digest\n"

m("VER-40 an edited spec stays trusted", T, HOLDS,
  "\t\treturn x.ProjectRoot == projectRoot && x.SpecPath == specPath && (x.SpecDigest == digest || true)\n",
  "TestST1d_DiscoveryReadsTheBinding|TestST1b_EditDropsTrust")
m("VER-40 a binding holds for another path", T, HOLDS,
  "\t\treturn x.ProjectRoot == projectRoot && (x.SpecPath == specPath || true) && x.SpecDigest == digest\n",
  "TestST1d_DiscoveryReadsTheBinding")
m("VER-40 a bindings file over its cap is read", T, "\tif len(data) > MaxTrustBytes {\n",
  "\tif len(data) > MaxTrustBytes && false {\n", "TestST1d_DiscoveryReadsTheBinding")

# The bindings file is a store file (D6): purge takes it and its replacement.
# Each entry is dropped; the inventory's rows are TRU-23's mutants.
P = "internal/purge/purge.go"
m("VER-40 purge leaves the bindings file", P, "\tspec.TrustFile,\n", "", "TestST1e_PurgeTakesTheBindingsFile")
m("VER-40 purge leaves the bindings replacement", P, "\t\".\" + spec.TrustFile + \".tmp-\",\n", "",
  "TestST1e_PurgeTakesTheBindingsFile")
