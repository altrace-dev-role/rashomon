# Card 4.3: the outcomes bundle (PF-SH1, PF-SH2). fired counts delivered and
# flag_only lines of the closed set and nothing else; a count under five prints
# "<5"; by_pattern is in the closed order and never null; the period spans the
# counted lines; the install digest has a domain of its own. Each guard is
# broken once (R4).
SO = "internal/share/outcomes.go"
SG = "internal/store/gaps.go"

m("ST-4.3 a holdout line counts as fired", SO,
  '\treturn *l.Delivery == steer.DeliveryDelivered || *l.Delivery == steer.DeliveryFlagOnly\n',
  '\treturn *l.Delivery == steer.DeliveryDelivered || *l.Delivery == steer.DeliveryFlagOnly || *l.Delivery == steer.DeliveryHoldout\n',
  "TestSH1_CountsFiredByDeliveryAndPattern")
m("ST-4.3 a pattern outside the closed set counts", SO,
  '\tif l.Delivery == nil || !pattern.Valid(l.Pattern) {\n',
  '\tif l.Delivery == nil {\n',
  "TestSH1_CountsFiredByDeliveryAndPattern")
m("ST-4.3 the <5 floor is at 4", SO,
  '\tif c < countFloor {\n',
  '\tif c < countFloor-1 {\n',
  "TestSH1b_UnderFivePrintsLess")
m("ST-4.3 by_pattern is not in the closed order", SO,
  '\tfor _, id := range pattern.All() {\n',
  '\tfor i := range pattern.All() {\n\t\tid := pattern.All()[len(pattern.All())-1-i]\n',
  "TestSH1_CountsFiredByDeliveryAndPattern")
m("ST-4.3 the period starts at the first counted line", SO,
  '\t\t\tif out.Counts.Fired == 1 || l.AtUnixMS < out.Period.FromUnixMS {\n',
  '\t\t\tif out.Counts.Fired == 1 {\n',
  "TestSH1_CountsFiredByDeliveryAndPattern")
m("ST-4.3 an empty by_pattern is null", SO,
  ', ByPattern: []PatternCount{}}\n',
  '}\n',
  "TestSH1c_EmptyStoreIsAnEmptyBundle")
m("ST-4.3 the bundle escapes < as HTML", SO,
  '\tenc.SetEscapeHTML(false)\n',
  '',
  "TestSH1_CountsFiredByDeliveryAndPattern")
m("ST-4.3 the install digest reuses forget-host's domain", SG,
  '\tm.Write([]byte("share-install\\x00"))\n',
  '\tm.Write([]byte("forget-host\\x00"))\n',
  "TestSH1c_EmptyStoreIsAnEmptyBundle")

# The CLI half (R2, R2c): the hash is of the bytes written, nothing is
# written unless all of it printed, consent and --out are required.
SC = "cmd/rashomon/share.go"

m("ST-4.3 the hash is of other bytes than the file's", SC,
  '\tif _, err := fmt.Fprintf(stdout, "%ssha256 %x\\n", b, sha256.Sum256(b)); err != nil {\n',
  '\tif _, err := fmt.Fprintf(stdout, "%ssha256 %x\\n", b, sha256.Sum256(b[:len(b)-1])); err != nil {\n',
  "TestSH2_PreviewEqualsWritten")
m("ST-4.3 the file is written before the print", SC,
  '\tif _, err := fmt.Fprintf(stdout, "%ssha256 %x\\n", b, sha256.Sum256(b)); err != nil {\n\t\treturn err\n\t}\n\treturn writeShareFile(out, b)\n',
  '\tif err := writeShareFile(out, b); err != nil {\n\t\treturn err\n\t}\n\t_, err = fmt.Fprintf(stdout, "%ssha256 %x\\n", b, sha256.Sum256(b))\n\treturn err\n',
  "TestSH2_PreviewEqualsWritten")
m("ST-4.3 the consent check is dropped", SC,
  '\tif !ok {\n',
  '\tif false && !ok {\n',
  "TestSH2c_RefusedWithoutConsentWritesNothing")
m("ST-4.3 --out is optional", SC,
  '\tif len(args) < len(verb) || out == "" {\n',
  '\tif len(args) < len(verb) {\n',
  "TestSH2d_RefusedWithoutOut")
