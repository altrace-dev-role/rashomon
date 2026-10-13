# LUI-11: notes are statements of fact. The second-person words and the
# imperatives are refused at render time by the same check STR-14 runs over
# the templates; a rendered order reaching the model is the whole regression.
m("LUI-11 the second person is no longer refused at render time", "internal/steer/render/templates.go",
  'var secondPerson = []string{"you", "your", "yours"}', 'var secondPerson = []string{}', "TestLUI11b_")
m("LUI-11 Validate stops running the statement check", "internal/steer/render/validate.go",
  "\treturn statementOfFact(unquoted)\n}\n", "\treturn nil\n}\n", "TestLUI11b_")
