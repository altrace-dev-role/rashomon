# VER-39: the spec digest is "sha256:" and the hex SHA-256 of the RFC 8785 form
# of the parsed document (internal/spec). Each break changes the canonical
# form or the digest around it; TestVER39_DigestCanonical's known answers were
# computed by a second route, so they judge every byte of the form.
T = "TestVER39_|TestVER39b_"
P = "internal/spec/parse.go"
TREE = "internal/spec/tree.go"

m("VER-39 the digest is over the raw bytes, not the canonical form", P, "sha256.Sum256(appendJCS(nil, root))", "sha256.Sum256(data)", T)
m("VER-39 the digest has no prefix", P, "digest := DigestPrefix + hex.EncodeToString(sum[:])", "digest := hex.EncodeToString(sum[:])", T)
m("VER-39 the digest is truncated", P, "hex.EncodeToString(sum[:])", "hex.EncodeToString(sum[:16])", T)
m("VER-39 object keys keep file order", TREE, "units[i], idx[i] = utf16.Encode([]rune(k)), i", "units[i], idx[i] = utf16.Encode([]rune(k)[:0]), i", T)
m("VER-39 object keys sort by code point, not UTF-16 unit", TREE, "slices.Compare(units[a], units[b])", "slices.Compare([]rune(keys[a]), []rune(keys[b]))", T)
m("VER-39 object keys sort in reverse", TREE, "slices.Compare(units[a], units[b])", "slices.Compare(units[b], units[a])", T)
m("VER-39 an object is written with a space after the colon", TREE, "\t\t\tdst = append(dst, ':')\n", "\t\t\tdst = append(dst, ':', ' ')\n", T)
m("VER-39 object members are written with a space after the comma", TREE,
  "\t\t\t\tdst = append(dst, ',')\n\t\t\t}\n\t\t\tdst = appendJCSString(dst, n.keys[j])", "\t\t\t\tdst = append(dst, ',', ' ')\n\t\t\t}\n\t\t\tdst = appendJCSString(dst, n.keys[j])", T)
m("VER-39 array elements are written with a semicolon", TREE,
  "\t\t\t\tdst = append(dst, ',')\n\t\t\t}\n\t\t\tdst = appendJCS(dst, v)", "\t\t\t\tdst = append(dst, ';')\n\t\t\t}\n\t\t\tdst = appendJCS(dst, v)", T)
m("VER-39 a false is written as true", TREE, "return strconv.AppendBool(dst, n.flag)", 'return append(dst, "true"...)', T)
m("VER-39 an integer is written in hexadecimal", TREE, "strconv.AppendInt(dst, n.num, 10)", "strconv.AppendInt(dst, n.num, 16)", T)
m("VER-39 an integer is written with a plus sign", TREE, "return strconv.AppendInt(dst, n.num, 10)", 'return append(strconv.AppendInt(append(dst, \'+\'), n.num, 10))', T)
m("VER-39 the quote is not escaped", TREE, "case c == '\"' || c == '\\\\':", "case c == '\\\\':", T)
m("VER-39 the backslash is not escaped", TREE, "case c == '\"' || c == '\\\\':", "case c == '\"':", T)
m("VER-39 backspace is written as \\u0008", TREE, "dst = append(dst, '\\\\', 'b')", "dst = append(dst, '\\\\', 'x')", T)
m("VER-39 form feed is written wrongly", TREE, "dst = append(dst, '\\\\', 'f')", "dst = append(dst, '\\\\', 'x')", T)
m("VER-39 newline is written wrongly", TREE, "dst = append(dst, '\\\\', 'n')", "dst = append(dst, '\\\\', 'x')", T)
m("VER-39 carriage return is written wrongly", TREE, "dst = append(dst, '\\\\', 'r')", "dst = append(dst, '\\\\', 'x')", T)
m("VER-39 tab is written wrongly", TREE, "dst = append(dst, '\\\\', 't')", "dst = append(dst, '\\\\', 'x')", T)
m("VER-39 a control below 0x1f is not escaped", TREE, "case c < 0x20:", "case c < 0x1f:", T)
m("VER-39 a control is written with upper-case hex", TREE, 'const hexDigits = "0123456789abcdef"', 'const hexDigits = "0123456789ABCDEF"', T)
m("VER-39 a control's low digit loses a bit", TREE, "hexDigits[c&0xf]", "hexDigits[c&0x7]", T)
m("VER-39 a control's high digit is wrong", TREE, "hexDigits[c>>4]", "hexDigits[c>>3]", T)
m("VER-39 DEL is escaped", TREE, "case c < 0x20:", "case c < 0x20 || c == 0x7f:", T)
m("VER-39 an HTML character is escaped", TREE, "case c == '\"' || c == '\\\\':", "case c == '\"' || c == '\\\\' || c == '<':", T)
m("VER-39 an object's braces are brackets", TREE, "\t\tdst = append(dst, '{')\n", "\t\tdst = append(dst, '[')\n", T)
m("VER-39 null is written as an empty string", TREE, 'return append(dst, "null"...)', 'return append(dst, `""`...)', T)
m("VER-39 an integer within 2^53-1 is clipped", TREE, "const maxSafeInt = 1<<53 - 1", "const maxSafeInt = 1<<52 - 1", T)
