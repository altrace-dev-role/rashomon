package jsonscan

import (
	"errors"
	"math"
)

// ErrNotArray is Elements' answer for a span that is not an array.
var ErrNotArray = errors.New("jsonscan: value is not an array")

// Elements calls fn with the span of each element of the array at arr, in
// order, until fn returns false. buf must have passed json.Valid, as Field
// requires; a span that is not an array is ErrNotArray. It locates each
// element and decodes nothing (RES-5).
func Elements(buf []byte, arr Span, fn func(Span) bool) error {
	if arr.Kind != Array || !arr.inside(buf) || len(buf) > math.MaxInt32 {
		return ErrNotArray
	}
	end := int(arr.Off + arr.Len)
	i := skipWS(buf, int(arr.Off)+1)
	for i < end && buf[i] != ']' {
		j, ok := skipValue(buf, i)
		if !ok || j > end {
			return ErrInvalid
		}
		if !fn(Span{Off: int32(i), Len: int32(j - i), Kind: kindOf(buf[i])}) {
			return nil
		}
		i = skipWS(buf, j)
		if i < end && buf[i] == ',' {
			i = skipWS(buf, i+1)
		}
	}
	return nil
}
