# errors-5 -- the reason tripwire lets the coverage record's verified branch pin
# reason to null, because null is the absence of a reason, not a code. Letting
# any const through would publish a reason code no test compares to the code.
m("errors-5 the tripwire accepts a const reason of any value", "test/acceptance/schema_constraint_test.go",
  '\t\tcase k == "const" && v == nil:\n', '\t\tcase k == "const" && (v == nil || v != nil):\n',
  "TestReasonVocabularyTripwireCatchesWhatTheComparisonSkips")
