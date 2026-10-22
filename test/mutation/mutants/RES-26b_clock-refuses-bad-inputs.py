# RES-26b: NewClock refuses a zero start and a non-positive soft budget,
# so a wiring bug that leaves either unset fails where it is made instead
# of reading as every phase timing out.
m("RES-26b NewClock stops refusing a zero start", "internal/budget/budget.go",
  "\tif start.IsZero() {\n\t\treturn Clock{}, errZeroStart\n\t}\n", "",
  "TestNewClockRefusesAZeroStartOrANonPositiveBudget")
m("RES-26b NewClock stops refusing a non-positive budget", "internal/budget/budget.go",
  "\tif soft <= 0 {\n\t\treturn Clock{}, errNonPositiveBudget\n\t}\n", "",
  "TestNewClockRefusesAZeroStartOrANonPositiveBudget")
