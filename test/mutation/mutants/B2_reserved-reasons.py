# B2 -- a reserved reason key is passed over by the reason tripwire only while
# it declares no enum; one that gains a vocabulary is unchecked until its
# comparison lands.
m("B2 a reserved reason key may declare a vocabulary unchecked", "test/acceptance/repo_test.go",
  "\t\tif reasonVocabularyPaths[p] || (reservedReasonPaths[p] && !vocabulary) {\n",
  "\t\tif reasonVocabularyPaths[p] || (reservedReasonPaths[p] && (vocabulary || !vocabulary)) {\n", "TestReasonVocabularyTripwire")
m("B2 a reserved reason path reads as compared", "test/acceptance/repo_test.go",
  "\t\t\t\tinto[p] = declaresEnum(child)\n", "\t\t\t\tinto[p] = false\n", "TestReasonVocabularyTripwire")
