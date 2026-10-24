# TR1 (Task 4.2): consent units. Installing grants record and nothing else;
# every other unit is off until asked; a unit grant is a unit_tty record that
# names one unit, was answered at a terminal and sits under an install.
m("TR1 record is not granted by an install", "internal/consent/units.go",
  "\tif u == UnitRecord {\n\t\treturn true\n\t}", "\tif u == UnitRecord {\n\t\treturn false\n\t}",
  "TestTR1b_RecordIsConsentedByInstall")
m("TR1 install grants every unit", "internal/consent/units.go",
  "\tif u == UnitRecord {\n\t\treturn true\n\t}", "\tif true {\n\t\treturn true\n\t}",
  "TestTR1c_OtherUnitsAreOffUntilAsked")
m("TR1 a unit grant needs no install under it", "internal/consent/units.go",
  "\tif _, ok := LastAccepted(recs); !ok {\n\t\treturn false\n\t}\n", "",
  "TestTR1c_OtherUnitsAreOffUntilAsked")
m("TR1 any unit grant turns on every unit", "internal/consent/units.go",
  "\t\tif r.Via == ViaUnitTTY && r.Unit == u {", "\t\tif r.Via == ViaUnitTTY {",
  "TestTR1c_OtherUnitsAreOffUntilAsked")
m("TR1 a name outside the closed set is a unit", "internal/consent/units.go",
  "\t_, ok := ParseUnit(string(u))\n\treturn ok\n", "\treturn u != \"\"\n",
  "TestTR1c_UnitRecordsAreRefusedUnlessWellFormed")
m("TR1 Has takes an unknown unit without complaint", "internal/consent/units.go",
  "\tif !u.valid() {\n\t\treturn false, fmt.Errorf", "\tif false {\n\t\treturn false, fmt.Errorf",
  "TestTR1b_RecordIsConsentedByInstall")
m("TR1 a unit grant needs no terminal", "internal/consent/record.go",
  "\t\treturn r.TTY && r.Unit.grantable()", "\t\treturn r.Unit.grantable()",
  "TestTR1c_UnitRecordsAreRefusedUnlessWellFormed")
m("TR1 record is a grantable unit", "internal/consent/units.go",
  "func (u Unit) grantable() bool { return u.valid() && u != UnitRecord }",
  "func (u Unit) grantable() bool { return u.valid() }",
  "TestTR1c_UnitRecordsAreRefusedUnlessWellFormed")
m("TR1 a unit rides on a record that is not a unit grant", "internal/consent/record.go",
  "\treturn r.Unit == \"\"\n}", "\treturn true\n}",
  "TestTR1c_OtherUnitsAreOffUntilAsked")
m("TR1 compaction keeps only the last unit grant", "internal/consent/record.go",
  "\t\tif latest[g.Unit] == i {", "\t\tif i == len(grants)-1 {",
  "TestTR1c_CompactionKeepsEveryUnitGrant")
m("TR1 compaction drops the unit grants", "internal/consent/record.go",
  "\t\tif latest[g.Unit] == i {", "\t\tif latest[g.Unit] == i && false {",
  "TestTR1c_CompactionKeepsEveryUnitGrant")
m("TR1 a unit grant moves the scope reader", "internal/consent/record.go",
  "\t\tcase ViaWatchTTY, ViaWatchYes, ViaConsentTTY, ViaConsentYes, ViaPluginEnable:",
  "\t\tcase ViaWatchTTY, ViaWatchYes, ViaConsentTTY, ViaConsentYes, ViaPluginEnable, ViaUnitTTY:",
  "TestTR1c_UnitGrantsDoNotMoveTheOtherReaders")
m("TR1 a unit's notice does not say how to agree", "internal/consent/units.go",
  "Agree with: rashomon consent share.outcomes\n", "",
  "TestTR1c_EveryUnitButRecordHasANotice")
m("TR1 a unit key on every record", "internal/consent/record.go",
  "`json:\"unit,omitempty\"`", "`json:\"unit\"`",
  "TestTR1b_PreUnitsRecordsMarshalAsTheyAlwaysDid")
m("TR1 consent takes two units", "cmd/rashomon/consent_unit.go",
  "\tif len(units) > 1 {", "\tif len(units) > 2 {",
  "TestTR1c_ConsentUnitRefusals")
m("TR1 record can be asked for", "cmd/rashomon/consent_unit.go",
  "\tif unit == consent.UnitRecord {", "\tif false {",
  "TestTR1c_ConsentUnitRefusals")
m("TR1 a unit is granted with no install", "cmd/rashomon/consent_unit.go",
  "\tif plan.installID == \"\" {\n\t\treturn errors.New(\"nothing is installed here, so there is nothing to agree to; rashomon watch shows the screen and installs\")\n\t}\n\tif plan.prior == nil {",
  "\tif plan.prior == nil {",
  "TestTR1c_ConsentUnitRefusals")
m("TR1 a unit is granted with no record consent", "cmd/rashomon/consent_unit.go",
  "\tif plan.prior == nil {\n\t\treturn errors.New(\"record is not on here", "\tif false {\n\t\treturn errors.New(\"record is not on here",
  "TestTR1c_ConsentUnitRefusals")
m("TR1 --yes answers a unit", "cmd/rashomon/consent_unit.go",
  "\tif !ans.tty {\n\t\treturn fmt.Errorf(refuseUnitYes, unit)\n\t}\n", "",
  "TestTR1c_ConsentUnitRefusals")
m("TR1 the grant hashes nothing", "cmd/rashomon/consent_unit.go",
  "\t\tTextSHA256:    consent.TextSHA256(notice),", "\t\tTextSHA256:    consent.TextSHA256(nil),",
  "TestTR1c_ConsentUnitAtATerminal")
m("TR1 the grant claims the current scope", "cmd/rashomon/consent_unit.go",
  "\t\tV:             consent.RecordVersion,\n\t\tSteer:", "\t\tV:             consent.RecordVersion,\n\t\tScopeVersion:  99,\n\t\tSteer:",
  "TestTR1c_ConsentUnitAtATerminal")
