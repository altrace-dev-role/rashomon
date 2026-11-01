# PRD-1: the PRD implementation truth table as a tested contract (card
# TRUTH-TABLE1). One break per check the card names for mutation: a cited
# file that is not there passes, a cited symbol the file does not declare
# passes, and a PRD id missing from the table passes. Each check's term is
# weakened, never deleted, so every name it binds stays in use and the mutant
# builds. The kill row is the self-test that feeds each defect to the checks.
# A fourth row empties H-113's contract allow list (D156) at its lookup, so no
# entry excuses anything (anchored on the lookup, not the entries, which gofmt
# re-aligns whenever one is added); H-113 itself then fails on the secret-shape
# findings.
P = "test/prd/truth_table_test.go"
A = "test/acceptance/h113_fixture_hygiene_test.go"

m("PRD-1 an absent evidence file passes", P,
  "\t\tif err != nil {\n\t\t\tfail(\"evidence\", \"%s: %v\", e.File, err)\n",
  "\t\tif err != nil && false {\n\t\t\tfail(\"evidence\", \"%s: %v\", e.File, err)\n",
  "TestPRD1b_EachCheckNamesItsRow")
m("PRD-1 an undeclared symbol passes", P,
  "\t\tif e.Symbol != \"\" && !declares(text, e.Symbol) {\n",
  "\t\tif e.Symbol != \"\" && !declares(text, e.Symbol) && false {\n",
  "TestPRD1b_EachCheckNamesItsRow")
m("PRD-1 a missing id passes", P, "\t\tif !seen[id] {\n", "\t\tif !seen[id] && false {\n",
  "TestPRD1b_EachCheckNamesItsRow")
m("PRD-1 the contract allow list is emptied", A,
  "\t\tif _, ok := h113Contracts[f.Path]; ok && f.What == \"a secret shape\" {\n",
  "\t\tif _, ok := h113Contracts[f.Path]; ok && f.What == \"a secret shape\" && false {\n",
  "TestH113_NoCommittedFixtureCarriesASecretOrAnAccountShape")
