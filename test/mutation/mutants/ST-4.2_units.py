# Card 4.2: consent per unit (PRD PF-SH1, PF-TR3). Has reads a unit's latest
# answer in file order and refuses a name that is no unit; a unit answer
# claims no install scope and survives compaction; the unit command shows,
# hashes and appends as the install screen does and refuses a unit nothing
# builds; SessionEnd copies a transcript only under the bodies grant. Each
# guard is broken once (R4).
CU = "internal/consent/units.go"
CR = "internal/consent/record.go"
CC = "cmd/rashomon/consent_unit.go"
HC = "internal/hook/consent_has.go"
HP = "internal/hook/probe.go"

m("ST-4.2 Has takes a unit's earliest answer", CU,
  '\tfor i := len(recs) - 1; i >= 0; i-- {\n\t\tswitch r := recs[i]; {\n',
  '\tfor i := 0; i < len(recs); i++ {\n\t\tswitch r := recs[i]; {\n',
  "TestTR1d_UnitOffIsTheLatestWord")
m("ST-4.2 Has ignores unit_on", CU,
  '\t\t\treturn r.UnitOn, nil\n',
  '\t\t\treturn true, nil\n',
  "TestTR1d_UnitOffIsTheLatestWord")
m("ST-4.2 a name that is no unit reads false", CU,
  '\t\treturn false, fmt.Errorf("consent: %q is no unit; the units are %s", unit, strings.Join(Units, ", "))\n',
  '\t\treturn false, nil\n',
  "TestTR1b_RecordIsConsentedByInstall")
m("ST-4.2 compaction keeps one answer per via", CR,
  '\t\tlatest[key{r.Via, r.ScopeVersion, r.Unit}] = i\n',
  '\t\tlatest[key{r.Via, r.ScopeVersion, ""}] = i\n',
  "TestTR1g_CompactionKeepsEachUnitsLatest")
m("ST-4.2 a unit answer may claim the install scope", CR,
  ' && r.ScopeVersion == 0 && r.RecordScope == ""',
  ' && r.RecordScope == ""',
  "TestTR1h_UnitAnswerIsNoInstallConsent")
m("ST-4.2 LastAccepted takes a unit answer", CR,
  '\t\tif recs[i].Unit != "" {\n\t\t\tcontinue // a unit answer is not the install screen\'s\n\t\t}\n',
  '',
  "TestTR1h_UnitAnswerIsNoInstallConsent")
m("ST-4.2 --show appends", CC,
  '\t\tif ans, err = askConsent("consent", shown, yes, show, workTreeQuestion{}, stdout, consent.QuestionConsent); err != nil || ans == nil {\n\t\t\treturn err\n\t\t}\n',
  '\t\tif ans, err = askConsent("consent", shown, yes, show, workTreeQuestion{}, stdout, consent.QuestionConsent); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif ans == nil {\n\t\t\tans = &answers{via: consent.ViaConsentYes}\n\t\t}\n',
  "TestTR1e_ConsentUnitShowsAndAppends")
m("ST-4.2 the unit record hashes nothing shown", CC,
  'TextSHA256: consent.TextSHA256(shown),',
  'TextSHA256: consent.TextSHA256(nil),',
  "TestTR1e_ConsentUnitShowsAndAppends")
m("ST-4.2 a unit nothing builds can be granted", CC,
  '\tif !slices.ContainsFunc(inv.Units, func(u disclose.UnitParagraph) bool { return u.Unit == unit }) {\n',
  '\tif false {\n',
  "TestTR1f_UnknownUnitIsRefused")
m("ST-4.2 SessionEnd copies without the bodies grant", HP,
  ' && bodiesConsented(st.Root()) {\n',
  ' {\n',
  "TestRC14m_NoBodiesConsentNoCopy")
m("ST-4.2 a probe without the consent reader copies", HC,
  '\tif ConsentHas == nil {\n\t\treturn false\n\t}\n',
  '\tif ConsentHas == nil {\n\t\treturn true\n\t}\n',
  "TestRC14m_NoBodiesConsentNoCopy")
