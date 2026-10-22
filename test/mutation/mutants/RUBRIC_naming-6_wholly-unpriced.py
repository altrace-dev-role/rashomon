# Rubric naming-6: Cost.Wholly read as "wholly priced", was true for a partly
# priced cost, and every caller negated it. WhollyUnpriced says what it
# answers; a cost with no responses behind it is a known zero, not unpriced.
m("RUBRIC naming-6 a cost with no responses reads as wholly unpriced", "internal/spend/spend.go",
  "func (c Cost) WhollyUnpriced() bool { return c.Priced == 0 && c.Unpriced > 0 }",
  "func (c Cost) WhollyUnpriced() bool { return c.Priced == 0 }",
  "TestRubricNAMING6_WhollyUnpricedIsTrueOnlyWithNoPricedPart")
m("RUBRIC naming-6 a partly priced cost reads as wholly unpriced", "internal/spend/spend.go",
  "func (c Cost) WhollyUnpriced() bool { return c.Priced == 0 && c.Unpriced > 0 }",
  "func (c Cost) WhollyUnpriced() bool { return c.Unpriced > 0 }",
  "TestRubricNAMING6_WhollyUnpricedIsTrueOnlyWithNoPricedPart")
