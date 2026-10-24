package core

func init() {
	registry["b"] = len(Double([]int{3, 3, 1}))
	registry["b2"] = len(Double([]int{7}))
	registry["b3"] = len(Double(nil))
}
