# TRU-23: install.json's replacement file, which SetRootInWorkTree writes,
# is in the inventory.
m("TRU-23 the install.json replacement drops out of the inventory", "internal/disclose/disclose.go",
  "\t\t\tPattern:    \".\" + store.FileInstallMeta + \".tmp-*\",\n", "\t\t\tPattern:    store.FileInstallMeta + \".tmp\",\n",
  "TestTRU23_EveryWrittenPathIsInTheInventory")
