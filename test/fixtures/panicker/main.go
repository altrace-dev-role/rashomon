// Command panicker panics without recovery and is expected to exit 2.
//
// It is the control for the exit-code assertions. Without it, a suite that
// asserts "every fault exited 0" cannot distinguish a working panic barrier
// from a harness that never observed an exit code at all -- and the second
// failure mode is silent, permanent, and indistinguishable from success.
package main

func main() {
	panic("control: this binary is supposed to exit 2")
}
