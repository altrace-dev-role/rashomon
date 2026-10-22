# DEL-25: admit counts every finding it refuses, so a producer sending
# findings outside the contract is visible later without Route's pinned
# body (TestDEL22f) having to change to carry the count itself.
m("DEL-25 admit stops counting a dropped finding", "internal/steer/route.go",
  "\t\t} else {\n\t\t\tdropped.Add(1)\n\t\t}\n",
  "\t\t}\n", "TestAdmitCountsWhatItDrops")
