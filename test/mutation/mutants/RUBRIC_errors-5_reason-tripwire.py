# errors-5 -- the reason tripwire lets the coverage record's verified branch pin
# reason to null, because null is the absence of a reason, not a code. Letting
# any const through would publish a reason code no test compares to the code.
m("errors-5 the tripwire accepts a const reason of any value", "test/acceptance/schema_constraint_test.go",
  '\t\tcase k == "const" && v == nil:\n', '\t\tcase k == "const" && (v == nil || v != nil):\n',
  "TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips")

# A reserved schema 5 reason key (reservedReasonPaths) is passed over only
# while it declares no vocabulary, an enum or a const; one that gains one is
# unchecked until its comparison lands.
m("errors-5 a reserved reason key may declare a vocabulary unchecked", "test/acceptance/repo_test.go",
  "\t\tif reasonVocabularyPaths[p] || (reservedReasonPaths[p] && !vocabulary) {\n",
  "\t\tif reasonVocabularyPaths[p] || (reservedReasonPaths[p] && (vocabulary || !vocabulary)) {\n",
  "TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips")
m("errors-5 a reserved reason path reads as compared", "test/acceptance/repo_test.go",
  "\t\t\t\tinto[p] = declaresEnum(child)\n", "\t\t\t\tinto[p] = false\n",
  "TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips")
m("errors-5 a reserved reason key may pin a const unchecked", "test/acceptance/repo_test.go",
  "\t\tif _, ok := v[\"const\"]; ok {\n\t\t\treturn true\n\t\t}\n", "",
  "TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips")
