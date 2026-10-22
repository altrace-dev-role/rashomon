# STR-13: a whole lowercase 40- or 64-hex path segment is kept only under
# "sha1" or "sha256" at that algorithm's length; anywhere else it is tested
# like any other segment, because the same shape is a hex token. Keeping it
# under any directory, at either length, or never is the regression.
m("STR-13 a 40- or 64-hex segment is kept under any directory", "internal/steer/render/slot/path.go",
  "\tswitch parent {\n\tcase \"sha1\":\n\t\tn = 40\n\tcase \"sha256\":\n\t\tn = 64\n\t}\n",
  "\tn = len(seg)\n\tif n != 40 && n != 64 {\n\t\tn = 0\n\t}\n", "TestSTR13j_")
m("STR-13 a digest directory admits either length", "internal/steer/render/slot/path.go",
  "\tcase \"sha1\":\n\t\tn = 40\n", "\tcase \"sha1\":\n\t\tn = len(seg)\n", "TestSTR13j_")
m("STR-13 a segment's parent directory is never tracked", "internal/steer/render/slot/path.go",
  "\t\tparent = seg\n", "", "TestSTR13j_")
