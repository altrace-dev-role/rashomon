package settings

import (
	"bytes"
	"testing"
)

// TestRubricLLD8_GetIsACopy: Get used to return the document's own bytes for
// a value, so a caller that wrote into them changed what Bytes renders.
func TestRubricLLD8_GetIsACopy(t *testing.T) {
	doc, err := Parse([]byte(`{"enabledPlugins":{"a@b":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	before := doc.Bytes()
	v, ok := doc.Get("enabledPlugins")
	if !ok {
		t.Fatal("premise: the key is not there")
	}
	for i := range v {
		v[i] = 'x'
	}
	if after := doc.Bytes(); !bytes.Equal(before, after) {
		t.Errorf("writing into Get's result changed the document:\nbefore %s\nafter  %s", before, after)
	}
}
