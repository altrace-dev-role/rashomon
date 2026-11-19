# A session whose only commands ran pytest has used Python's tooling, so its
# pytest calls count under the python family and python is not listed as not
# exercised. Dropping pytest from the table puts python back on the
# not-exercised line after two pytest runs.
m("families drops pytest from python", "internal/report/families.go",
  "\t\t\t\"uv\": \"python\", \"poetry\": \"python\", \"pytest\": \"python\",\n",
  "\t\t\t\"uv\": \"python\", \"poetry\": \"python\",\n",
  "TestFamPy1_APytestRunExercisesPython")
