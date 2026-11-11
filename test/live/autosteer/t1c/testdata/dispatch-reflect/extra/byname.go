package extra

import "reflect"

// Perimeter is exported, declared by no interface and called by no static
// site: a static reader counts zero callers. ByName below can reach it.
func (r Rect) Perimeter() float64 { return 2 * (r.W + r.H) }

// ByName calls a method chosen at run time, a boundary no static reader can
// resolve: whatever the static count says, the true fan-in is unknown.
func ByName(v any, name string) []reflect.Value {
	return reflect.ValueOf(v).MethodByName(name).Call(nil)
}

// Probe reaches Rect.Perimeter only through ByName.
func Probe() []reflect.Value { return ByName(Rect{W: 1, H: 2}, "Perimeter") }
