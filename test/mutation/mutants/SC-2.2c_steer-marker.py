# Card 2.2c: the steer-disclosure marker (D49). consent.json stays the record
# of truth; store.FileSteerDisclosure is a view of it holding the highest
# steer disclosure on file, an integer the reading build compares with its
# own version. Every consent write removes it before the append and writes
# it again after, every session start puts it right, and a failure leaves it
# absent, which reads 0, no consent. The kill row is the named test's.
S = "internal/store/steerdisclosure.go"
R = "internal/consent/record.go"
H = "internal/hook/steerdisclosure.go"
P = "internal/hook/probe.go"
U = "internal/purge/purge.go"
K = "cmd/rashomon/consent.go"

# The marker in the store.
m("SC-2.2c malformed content read as a version", S,
  '\tif s == "" || strings.Trim(s, "0123456789") != "" {\n', '\tif s == "" {\n', "TestSC1_")
m("SC-2.2c a negative write accepted", S,
  '\tif n < 0 {\n\t\treturn fmt.Errorf("store: refusing', '\tif false {\n\t\treturn fmt.Errorf("store: refusing',
  "TestSC1_")

# consent: the reader and Append's two views.
m("SC-2.2c SteerDisclosure takes the latest record's value, not the highest", R,
  '\tbest, _ := maxBy(recs, func(r Record) int { return r.SteerDisclosure })\n',
  '\tbest := 0\n\tif len(recs) > 0 {\n\t\tbest = recs[len(recs)-1].SteerDisclosure\n\t}\n', "TestSC2_")
m("SC-2.2c a negative highest reaches the marker", R,
  '\treturn max(best, 0), nil\n', '\treturn best, nil\n', "TestSC2_")
m("SC-2.2c the steer marker written before the append, leading the log", R,
  '\tif err := appendRecord(root, line); err != nil {\n\t\treturn err\n\t}\n\tvar scopeErr, steerErr error\n'
  '\tif err := syncMarker(root); err != nil {\n\t\tscopeErr = fmt.Errorf("%w: %w", ErrScopeMarker, err)\n\t}\n'
  '\tif err := syncSteerMarker(root); err != nil {\n\t\tsteerErr = fmt.Errorf("%w: %w", ErrSteerMarker, err)\n\t}\n',
  '\tvar scopeErr, steerErr error\n'
  '\tif err := syncSteerMarker(root); err != nil {\n\t\tsteerErr = fmt.Errorf("%w: %w", ErrSteerMarker, err)\n\t}\n'
  '\tif err := appendRecord(root, line); err != nil {\n\t\treturn err\n\t}\n'
  '\tif err := syncMarker(root); err != nil {\n\t\tscopeErr = fmt.Errorf("%w: %w", ErrScopeMarker, err)\n\t}\n',
  "TestSC3_")
m("SC-2.2c Append skips the steer marker's remove", R,
  '\tif err := store.RemoveSteerDisclosureMarker(root); err != nil {\n\t\treturn err\n\t}\n', '', "TestSC3b_")
m("SC-2.2c a failed record-scope write stops the steer marker's", R,
  '\t\tscopeErr = fmt.Errorf("%w: %w", ErrScopeMarker, err)\n',
  '\t\treturn fmt.Errorf("%w: %w", ErrScopeMarker, err)\n', "TestSC3b_")
m("SC-2.2c a failed steer write wrapped as the record scope's", R,
  '\t\tsteerErr = fmt.Errorf("%w: %w", ErrSteerMarker, err)\n',
  '\t\tsteerErr = fmt.Errorf("%w: %w", ErrScopeMarker, err)\n', "TestSC3b_")

# hook: the session-start sync.
m("SC-2.2c the steer sync writes when equal", H,
  '\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {\n',
  '\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) && false {\n', "TestSC4_")
m("SC-2.2c the steer sync only raises the marker", H,
  '\tif have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {\n',
  '\tif have >= want && (err == nil || errors.Is(err, fs.ErrNotExist)) {\n', "TestSC6_")
m("SC-2.2c a reader error leaves the stale marker in place", H,
  '\tif err != nil || want < 0 {\n\t\t_ = store.RemoveSteerDisclosureMarker(root)\n\t\treturn false\n\t}\n',
  '\tif err != nil || want < 0 {\n\t\treturn false\n\t}\n', "TestSC4_")
m("SC-2.2c the probe skips the steer sync", P,
  '\t\t\tsyncSteerDisclosure(st.Root())\n', '', "TestSC4b_")
m("SC-2.2c a failed steer sync names the record scope's reason", P,
  '\t\t\tsyncSteerDisclosure(st.Root())\n',
  '\t\t\tif !syncSteerDisclosure(st.Root()) {\n\t\t\t\treason = store.ReasonRecordScopeUnresolved\n\t\t\t}\n',
  "TestSC4b_")
m("SC-2.2c a failed record-scope sync skips the steer sync", P,
  '\t\t\t\treason = store.ReasonRecordScopeUnresolved\n\t\t\t}\n\t\t\tsyncSteerDisclosure(st.Root())\n',
  '\t\t\t\treason = store.ReasonRecordScopeUnresolved\n\t\t\t\treturn nil\n\t\t\t}\n\t\t\tsyncSteerDisclosure(st.Root())\n',
  "TestSC4b_")

# The consent command and the inventories.
m("SC-2.2c the consent command says nothing was written for an unwritten steer marker", K,
  '\tcase errors.Is(err, consent.ErrSteerMarker):\n', '\tcase false:\n', "TestSC5_")
m("SC-2.2c the binary hands the probe no steer reader", K,
  '\thook.ConsentSteerDisclosure = consent.SteerDisclosure\n', '', "TestSC6_")
m("SC-2.2c purge does not list the steer marker", U,
  '\tstore.FileRecordScope, store.FileSteerDisclosure,\n', '\tstore.FileRecordScope,\n', "TestTRU28_")
m("SC-2.2c purge does not list the steer marker's temporary file", U,
  '\t"." + store.FileSteerDisclosure + ".tmp-", "." + store.FileHarnesses + ".tmp-",\n',
  '\t"." + store.FileHarnesses + ".tmp-",\n', "TestTRU28_")
