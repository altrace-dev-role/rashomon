# test-11: where locks are mandatory (Windows) a held lock refuses an unlocked
# read. ReadRunConsistent's fallback read and ReadGaps count that refusal as
# one line they could not read, so the session reads unverified instead of
# digest.Build failing; a read error after the lock was taken is returned.
# A directory read (EISDIR) stands in for the refusal on unix, where the
# sweep runs; the Windows OS fact itself is CI's lock-windows job.
m("test-11 a refused read is not counted", "internal/store/read.go",
  "\t\tfn(nil)\n\t\treturn nil\n", "\t\treturn nil\n",
  "TestRubricTEST11_ARefusedReadUnderAHeldLockCountsAsSkipped|TestRubricTEST11_ARefusedGapsReadCountsAsSkipped")
m("test-11 a refusal is tolerated with the lock taken", "internal/store/read.go",
  "\tif lerr != nil {\n\t\treturn refusedAsSkipped(err, refused, fn)\n",
  "\tif true {\n\t\treturn refusedAsSkipped(err, refused, fn)\n",
  "TestRubricTEST11_ARefusalWithTheLockTakenIsAnError")
m("test-11 the gaps read fails on a refused read", "internal/store/read.go",
  "\terr = refusedAsSkipped(eachLine(path, fn), refused, fn)\n", "\terr = eachLine(path, fn)\n",
  "TestRubricTEST11_ARefusedGapsReadCountsAsSkipped")
m("test-11 the try-lock read ignores its refusal predicate", "internal/store/read.go",
  "\t\treturn refusedAsSkipped(err, refused, fn)\n", "\t\treturn refusedAsSkipped(err, readRefused, fn)\n",
  "TestRubricTEST11_ARefusedReadUnderAHeldLockCountsAsSkipped")
