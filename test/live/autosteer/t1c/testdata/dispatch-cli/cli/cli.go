package cli

import (
	"example.test/dispatch/app"
	"example.test/dispatch/shape"
)

// Run is a caller of app from another package: the blast-radius read names
// the packages calling into an edited one, and app had none.
func Run() float64 {
	return app.Total([]shape.Shape{shape.Circle{R: 1}}) + app.RunAll()
}
