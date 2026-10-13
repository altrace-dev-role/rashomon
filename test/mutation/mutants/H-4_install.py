m("H-4  installer assigns instead of appends (foreign entries dropped)", "internal/install/install.go",
  "\t\t\tif Owner(e, event) != spec.InstallID {\n\t\t\t\tout = append(out, e)\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif detail := intact(e, event); detail != \"\" {\n\t\t\t\t// Someone",
  "\t\t\tif Owner(e, event) != spec.InstallID {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif detail := intact(e, event); detail != \"\" {\n\t\t\t\t// Someone", "TestH4_")
