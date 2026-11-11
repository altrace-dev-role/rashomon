package app

import (
	"strconv"

	"example.com/shapes/shape"
)

// Holder embeds a Shape.
type Holder struct{ shape.Shape }

// Run builds a square and sums it.
func Run() string {
	return strconv.Itoa(shape.Sum([]shape.Shape{&shape.Square{N: 2}}))
}

var Default = Holder{}
