# RMT-1: a prompt taken into the running turn is no turn end (card
# REC-MIDTURN1): `recap pending` with the running turn's prompt_id prints no
# catch-up and claims nothing, so the turn's Stop speaks for it; the next
# turn's prompt and a payload with no prompt_id keep the catch-up. Each break
# removes one rule and names the tests that hold it.
RT = "cmd/rashomon/recap_turn.go"
G = "\t\tunchecked = unchecked && pp.PromptID != d.PromptID\n"
m("RMT-1 a queued prompt catches up its own turn", RT, G, "", "TestRMT1a_|TestRMT1b_")
m("RMT-1 no prompt catches up", RT, G, "\t\tunchecked = false\n", "TestRMT1c_|TestH106_")
m("RMT-1 a prompt with no id reads as the running turn's", RT, G,
  "\t\tunchecked = unchecked && pp.PromptID != \"\" && pp.PromptID != d.PromptID\n", "TestRMT1d_|TestH106_")
