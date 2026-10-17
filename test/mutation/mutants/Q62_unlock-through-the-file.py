# lockFile's unlock released a descriptor by number, so an unlock that ran
# after its file was closed released whichever file had been given that
# number since. It releases through the *os.File now, which refuses once
# closed.
m("unix lockFile's unlock releases a descriptor by number again", "internal/store/lock_unix.go",
  "\t\t\treturn func() { unlockFile(f) }, waited, nil\n",
  "\t\t\treturn func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }, waited, nil\n",
  "TestUnlockAfterCloseReleasesNothing")
