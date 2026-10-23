#!/bin/sh
# The hostile inputs spec item 3 lists by name, one directory per case beside a clean ok.py.
# Build as the scanning user: sh hostile_visible.sh <root>
set -eu
R="$1"
mk() { mkdir -p "$R/$1"; printf 'x = 1\n' > "$R/$1/ok.py"; }
mk unreadable;      printf 'x = 1\n' > "$R/unreadable/t.py"; chmod 000 "$R/unreadable/t.py"
mk dangling;        ln -s missing.py "$R/dangling/t.py"
mk symlink_loop;    mkdir "$R/symlink_loop/d"; ln -s .. "$R/symlink_loop/d/up"
mk nul_byte;        printf 'x = 1\000\n' > "$R/nul_byte/t.py"
mk deep_nesting;    python -c "print('x = ' + '(' * 150 + '1' + ')' * 150)" > "$R/deep_nesting/t.py"
mk non_utf8_name;   printf 'x = 1\n' > "$R/non_utf8_name/$(printf 'caf\351.py')"
mk bom;             printf '\357\273\277x = 1\n' > "$R/bom/t.py"
mk crlf;            printf 'x = 1\r\ny = 2\r\n' > "$R/crlf/t.py"
mk empty;           : > "$R/empty/t.py"
mk invalid_coding;  printf '# -*- coding: no-such-codec -*-\nx = 1\n' > "$R/invalid_coding/t.py"
mk dir_named_py;    mkdir "$R/dir_named_py/pkg.py"; printf 'x = 1\n' > "$R/dir_named_py/pkg.py/m.py"
