# TRU-23: consent.json and its compaction target are in the inventory, read
# from the store's own name for the file.
m("TRU-23 consent.json drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    store.FileConsent,\n", "\t\t\tPattern:    \"consents.json\",\n",
  "TestTRU23_ConsentWritesAreInTheInventory")
m("TRU-23 the consent compaction target drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    \".\" + store.FileConsent + \".tmp-*\",\n", "\t\t\tPattern:    store.FileConsent + \".tmp\",\n",
  "TestTRU23_ConsentWritesAreInTheInventory")
m("TRU-23 a new consent write site goes uncounted", "internal/consent/record.go",
  "\tf, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)\n",
  "\tf, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)\n\t_, _ = os.Create(path + \".bak\")\n",
  "TestTRU23_WriteCallSitesArePinned")
