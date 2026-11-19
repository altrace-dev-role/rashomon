# s3pin.sh: sourced by smoke.sh at the re-cut (pre-run amendment t2-prereg-amend-s1-s3-2026-11-12.txt, part B).
# s3pin ARMDIR PINFILE LABEL: the lines of ARMDIR/index.txt then ARMDIR/knowledge.txt that match smoke.sh:112's
# pattern, compared by content with PINFILE's lines that are neither comments nor blank, both LC_ALL=C sorted.
# A pinned line not printed: one "S-3 MISSING LABEL: <line>" line each, no stop. A printed line outside the pin:
# the lines are shown and the smoke stops with exit 3 (r3 S-3: the line goes to the director). Writes ARMDIR.s3got
# and ARMDIR.s3pin beside ARMDIR. Needs NAME and fail() from smoke.sh.
s3pin() {
  { [ -f "$1/index.txt" ] && [ -f "$1/knowledge.txt" ]; } || fail "S-3 $3: $1 has no index.txt or knowledge.txt"
  [ -s "$2" ] || fail "S-3 $3: the pin $2 is missing or empty"
  cat "$1/index.txt" "$1/knowledge.txt" | grep -i -E 'error|refus|invalid|cannot' | LC_ALL=C sort > "$1.s3got"
  grep -v -e '^#' -e '^$' "$2" | LC_ALL=C sort > "$1.s3pin"
  LC_ALL=C comm -23 "$1.s3pin" "$1.s3got" | sed "s/^/S-3 MISSING $3: /"
  s3x=$(LC_ALL=C comm -13 "$1.s3pin" "$1.s3got")
  if [ -n "$s3x" ]; then
    echo "== S-3 $3: $(printf '%s\n' "$s3x" | wc -l | tr -d ' ') line(s) outside the pin $2:"
    printf '%s\n' "$s3x" | head -10 | cut -c1-300 | sed 's/^/  /'
    echo "SMOKE STOP $NAME: S-3 $3 printed a line outside its pin; the line goes to the director, no rule is stripped"
    exit 3
  fi
  echo "== S-3 $3: $(wc -l < "$1.s3got" | tr -d ' ') line(s), every one pinned ($2)"
}
