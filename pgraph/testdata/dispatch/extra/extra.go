package extra

// Rect satisfies shape.Shape and is never used by the module's own code.
type Rect struct{ W, H float64 }

func (r Rect) Area() float64 { return r.W * r.H }

// seal is spelled like shape's unexported method and is a different method.
func (Rect) seal() string { return "rect" }

// Dog has a Name method and so satisfies shape.Namer.
type Dog struct{}

func (Dog) Name() string { return "dog" }
