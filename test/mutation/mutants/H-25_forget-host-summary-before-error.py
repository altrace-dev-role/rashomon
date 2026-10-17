# forget --host stopped at the first of its two steps that failed, before
# printing anything: a baseline that cannot be read swallowed the summary of
# what ForgetHost had already removed. The summary now prints before either
# error is returned.
m("forgetHost returns a partial ForgetHost error before printing", "cmd/rashomon/main.go",
  "\tgaps, forgetErr := st.ForgetHost(host, time.Now())",
  "\tgaps, forgetErr := st.ForgetHost(host, time.Now())\n\tif forgetErr != nil {\n\t\treturn forgetErr\n\t}",
  "TestForgetHost_PrintsTheSummaryEvenWhenForgetHostFails")
m("forgetHost returns a clearBaselines error before printing", "cmd/rashomon/main.go",
  "\tcleared, hadBaselines, clearErr := clearBaselines(st.Root(), host)",
  "\tcleared, hadBaselines, clearErr := clearBaselines(st.Root(), host)\n\tif clearErr != nil {\n\t\treturn clearErr\n\t}",
  "TestForgetHost_PrintsTheSummaryEvenWhenClearBaselinesFails")
