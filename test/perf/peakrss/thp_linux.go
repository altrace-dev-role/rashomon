//go:build linux

package peakrss

import (
	"fmt"
	"syscall"
)

// prctl options (linux/prctl.h): PR_SET_THP_DISABLE and PR_GET_THP_DISABLE.
const (
	prSetTHPDisable = 41
	prGetTHPDisable = 42
)

// DisableTHP turns transparent huge pages off for this process and, because
// the flag lives in the address space's flags and survives fork and exec, for
// every process it starts afterwards: the reaper and the child it measures.
//
// Why a harness wants it. With /sys/kernel/mm/transparent_hugepage/enabled at
// "always" (the ubuntu-24.04 runners; this repository's reference VM reads
// "madvise") the kernel backs the Go heap, the binary's text and its stacks
// with 2 MiB pages when a fault finds one available, and a reading moves in
// 2 MiB steps with whether it did: hook.400B read 6.1, 8.1 and 10.2 MiB on
// the same binary on the VM once "always" was set there, and the same eight
// entries and the 1 MiB cases read as CI's did. With "never" they read one
// number (about 5.0 for hook.400B). A user's machine may be either, so this
// is a choice of what the gate measures: the 4 KiB-page footprint, which is
// what "madvise" and "never" hosts, and Go's own default (the runtime does not
// ask for huge pages), give. It does not hold a user's THP=always host to a
// number that depends on the kernel's page availability at that moment.
//
// It reports an error rather than measuring with the flag off by accident:
// the caller refuses to run.
func DisableTHP() error {
	if _, err := prctl(prSetTHPDisable, 1); err != nil {
		return fmt.Errorf("peakrss: prctl(PR_SET_THP_DISABLE): %w", err)
	}
	r, err := prctl(prGetTHPDisable, 0)
	if err != nil {
		return fmt.Errorf("peakrss: prctl(PR_GET_THP_DISABLE): %w", err)
	}
	if r != 1 {
		return fmt.Errorf("peakrss: prctl(PR_SET_THP_DISABLE) did not take: PR_GET_THP_DISABLE read %d", r)
	}
	return nil
}

// prctl is the one raw call this file makes (test/acceptance's H-17 scan
// names it): the syscall package has no wrapper for prctl, and Syscall6 is
// what passes the zero arguments PR_SET_THP_DISABLE requires, where
// Syscall would leave two registers unset. It opens no socket and starts no
// process.
func prctl(option, arg uintptr) (uintptr, error) {
	r, _, e := syscall.Syscall6(syscall.SYS_PRCTL, option, arg, 0, 0, 0, 0)
	if e != 0 {
		return 0, e
	}
	return r, nil
}
