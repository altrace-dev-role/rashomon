# CONSENT-SCOPE1: the consent screen says what record scope v5 keeps, and
# the steer disclosure moves past alpha.1's 2, so an alpha.1 consent's notes
# wait for that screen. The kill rows are the named tests'.
D = "internal/steer/disclosure.go"
P = "internal/disclose/paragraphs.go"

m("CSCOPE-1 the disclosure stays at alpha.1's", D,
  "const DisclosureVersion = 3\n", "const DisclosureVersion = 2\n", "TestCSCOPE1b_")
m("CSCOPE-1 the screen is silent on the written file's digest", P,
  ", and the SHA-256 of the file an edit names as the call left it (never its content)", "", "TestCSCOPE1a_")
