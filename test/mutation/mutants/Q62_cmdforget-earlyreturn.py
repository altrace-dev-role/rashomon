m("cmdForget claims records were forgotten after the listing failed outright", "cmd/rashomon/forget.go",
  "\tgaps, forgetErr := st.ForgetWindow(from, to, time.Now())\n\tif forgetErr != nil && !errors.Is(forgetErr, store.ErrForgetIncomplete) {\n\t\treturn forgetErr\n\t}\n\ttotal := 0\n",
  "\tgaps, forgetErr := st.ForgetWindow(from, to, time.Now())\n\ttotal := 0\n",
  "TestCmdForget_SaysNothingWasForgottenWhenTheListingFailed")
