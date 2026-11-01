#!/bin/sh
# Candidate hostile-input cases beyond the 11 visible and 16 hidden ones (second opinion, 2026-10-22).
# HIDDEN CLASS: never show this file or its case names to any py27-* session. Each case is a directory
# holding one ordinary ok.py beside the hostile entry, in the same form as hostile_holdout.sh.
# Build as the scanning user, inside a Linux container: sh hostile_extra.sh <root>
set -eu
R="$1"
mk() { mkdir -p "$R/$1"; printf 'x = 1\n' > "$R/$1/ok.py"; }
# --- entries that are not regular files
mk socket_py;        python -c "import socket; s = socket.socket(socket.AF_UNIX); s.bind('$R/socket_py/t.py')"
mk devnull_link;     ln -s /dev/null "$R/devnull_link/t.py"
mk link_to_root;     ln -s / "$R/link_to_root/d"                              # a walker that follows it never ends
mk symlink_chain;    i=1; while [ $i -lt 60 ]; do ln -s "a$((i + 1)).py" "$R/symlink_chain/a$i.py"; i=$((i + 1)); done; printf 'x = 1\n' > "$R/symlink_chain/a60.py"   # ELOOP at 40
mk hardlink_pair;    printf 'print 1\n' > "$R/hardlink_pair/a.py"; ln "$R/hardlink_pair/a.py" "$R/hardlink_pair/b.py"
mk unlistable_py;    mkdir "$R/unlistable_py/pkg.py"; printf 'x = 1\n' > "$R/unlistable_py/pkg.py/m.py"; chmod 000 "$R/unlistable_py/pkg.py"
mk pkg_named_py;     mkdir "$R/pkg_named_py/pkg.py"; printf '' > "$R/pkg_named_py/pkg.py/__init__.py"; printf 'print 1\n' > "$R/pkg_named_py/pkg.py/m.py"
mk pyc_only;         printf 'junk' > "$R/pyc_only/m.pyc"
# --- names
mk dot_py_name;      printf 'x = 1\n' > "$R/dot_py_name/.py"
mk upper_ext;        printf 'print 1\n' > "$R/upper_ext/t.PY"
mk trailing_space;   printf 'print 1\n' > "$R/trailing_space/t.py "
mk leading_dash;     printf 'print 1\n' > "$R/leading_dash/-t.py"
mk case_pair;        printf 'print 1\n' > "$R/case_pair/A.py"; printf 'print 2\n' > "$R/case_pair/a.py"
mk very_deep_dirs;   d="$R/very_deep_dirs"; i=0; while [ $i -lt 1800 ]; do d="$d/d"; i=$((i + 1)); done; mkdir -p "$d"; printf 'print 1\n' > "$d/t.py"   # about 3,600 chars, under PATH_MAX
mk many_files;       i=0; while [ $i -lt 2000 ]; do printf 'x = %d\n' $i > "$R/many_files/f$i.py"; i=$((i + 1)); done
# --- encodings and line endings
mk utf16_bom;        printf '\377\376x\000 \000=\000 \0001\000\n\000' > "$R/utf16_bom/t.py"
mk non_ascii_no_cookie; printf 'x = "caf\351"\n' > "$R/non_ascii_no_cookie/t.py"
mk utf8_sig_cookie;  printf '# -*- coding: utf-8-sig -*-\nx = 1\n' > "$R/utf8_sig_cookie/t.py"
mk cookie_on_line_3; printf '#!/usr/bin/env python\n#\n# -*- coding: latin-1 -*-\nx = "caf\351"\n' > "$R/cookie_on_line_3/t.py"
mk bom_latin1_cookie; printf '\357\273\277# -*- coding: latin-1 -*-\nx = 1\n' > "$R/bom_latin1_cookie/t.py"
mk mixed_newlines;   printf 'x = 1\r\ny = 2\rz = 3\n' > "$R/mixed_newlines/t.py"
mk formfeed_indent;  printf 'if 1:\n\014    x = 1\n' > "$R/formfeed_indent/t.py"
mk only_shebang;     printf '#!/usr/bin/env python\n' > "$R/only_shebang/t.py"
mk continuation_eof; printf 'x = 1 + \\' > "$R/continuation_eof/t.py"
mk unterminated_triple; printf 'x = """abc\n' > "$R/unterminated_triple/t.py"
mk nesting_1000;     python -c "print('x = ' + '(' * 1000 + '1' + ')' * 1000)" > "$R/nesting_1000/t.py"   # the scanner's own recursion
mk nesting_20;       python -c "print('x = ' + '(' * 20 + '1' + ')' * 20)" > "$R/nesting_20/t.py"
# --- rule precision (no finding wanted) and judgment twins
mk ne_in_string;     printf 'x = "a <> b"\n' > "$R/ne_in_string/t.py"
mk backtick_in_string; printf 'x = "`a`"\n' > "$R/backtick_in_string/t.py"
mk long_in_string;   printf 'x = "10L"\n' > "$R/long_in_string/t.py"
mk except_comma_in_string; printf 'x = "except X, e:"\n' > "$R/except_comma_in_string/t.py"
mk except_comma_nested; printf 'try:\n    pass\nexcept (A, (B, C)), e:\n    pass\n' > "$R/except_comma_nested/t.py"
mk except_comma_continued; printf 'try:\n    pass\nexcept ValueError, \\\n        e:\n    pass\n' > "$R/except_comma_continued/t.py"
mk future_division;  printf 'from __future__ import division\nx = 1 / 2\n' > "$R/future_division/t.py"
mk print_tuple;      printf 'print(1, 2)\n' > "$R/print_tuple/t.py"          # compiles on both; tuple on 2.7, two args on 3
mk print_single;     printf 'print(1)\n' > "$R/print_single/t.py"            # compiles on both; same output (the D5 twin)
mk exec_call_form;   printf 'exec("x = 1")\n' > "$R/exec_call_form/t.py"     # statement on 2.7, call on 3; same effect
mk octal_zero;       printf 'x = 00\n' > "$R/octal_zero/t.py"
mk raise_three;      printf 'raise ValueError, "x", None\n' > "$R/raise_three/t.py"
mk has_key;          printf 'd = {}\nd.has_key(1)\n' > "$R/has_key/t.py"      # compiles on both; fails at runtime on 3
mk br_prefix;        printf 'x = br"a"\n' > "$R/br_prefix/t.py"
mk ur_with_unicode_literals; printf 'from __future__ import unicode_literals\nx = ur"a"\n' > "$R/ur_with_unicode_literals/t.py"
mk tabs_only;        printf 'if 1:\n\tx = 1\n\ty = 2\n' > "$R/tabs_only/t.py"
chmod -R a+rX "$R" 2>/dev/null || true; chmod 000 "$R/unlistable_py/pkg.py"
