m("H-5  absent settings file is an error", "internal/settings/document.go",
  "\tif errors.Is(err, fs.ErrNotExist) {\n\t\treturn &Document{}, nil\n\t}\n\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn ParseNamed(path, data)",
  "\tif err != nil {\n\t\treturn nil, err\n\t}\n\t_ = errors.Is\n\t_ = fs.ErrNotExist\n\treturn ParseNamed(path, data)", "TestH5_")
