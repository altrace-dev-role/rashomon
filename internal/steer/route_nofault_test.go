//go:build !rashomonfault

package steer

import "testing"

// str8BarrierFault runs only in the fault build (-tags rashomonfault), where
// the store's barrier can be made to fail.
func str8BarrierFault(t *testing.T) {
	t.Log("the barrier fault case runs under -tags rashomonfault")
}
