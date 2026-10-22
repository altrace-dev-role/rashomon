# LUI-4: Owner recognises a listed legacy subcommand as ours, so a rename
# refreshes an old entry in place instead of leaving it behind as residue
# (detach would otherwise never recognise it either). Dropping legacyForms
# from the candidate list is the whole regression: TestLUI4_ seeds a
# legacy-shaped entry, so the refresh never fires, and Stop ends up with
# three entries where it should have two.
m("LUI-4 Owner never tries a listed legacy subcommand", "internal/install/install.go",
  "\tforms := append([]string{subcommand(event)}, legacyForms[event]...)\n",
  "\tforms := []string{subcommand(event)}\n",
  "TestLUI4_")
