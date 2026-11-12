package main

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// ubGolden is the v1 unit-brief file for ubBrief under a fixed header: the
// format's bytes. The fact objects carry Go field names with no json tags,
// so every field is written and a renamed, reordered or newly tagged field
// changes these bytes. A unit brief holds code and knowledge lines only
// (UnitBrief's consumers and reuse line are file or symbol keys, its rules
// and lessons knowledge), so CodeFact, KnowledgeFact and Counts are the
// shapes a file can carry.
const ubGolden = `{"v":1,"unit":"g:r1#unit:py:store","first":"g:r1#file:brief/brief.py","tier":"algorithmic","rev":"50942026abcd","prompt":"prompt-7","lines":[{"key":"file:brief/brief.py","code":{"Path":"brief/brief.py","Role":"consumer","Basis":"","Band":"","Names":["b1","b2","b3"],"Consts":[],"Calls":[],"Writes":[],"Reads":[],"Imports":[],"Lessons":[]},"more":{"Names":2,"Consts":0,"Calls":0,"Writes":0,"Reads":0,"Imports":0,"Lessons":0,"Writers":0,"Readers":0,"Consumers":0,"Records":0}},{"key":"dec:D1","knowledge":{"Kind":"decision","ID":"dec:D1","Band":"","Path":"store/loader.py","Title":"","Clause":"rows load in key order","Source":""},"more":{"Names":0,"Consts":0,"Calls":0,"Writes":0,"Reads":0,"Imports":0,"Lessons":0,"Writers":0,"Readers":0,"Consumers":0,"Records":0}},{"key":"les:L1","knowledge":{"Kind":"lesson","ID":"les:L1","Band":"","Path":"","Title":"a reload dropped rows","Clause":"","Source":"incidents/L1.md"},"more":{"Names":0,"Consts":0,"Calls":0,"Writes":0,"Reads":0,"Imports":0,"Lessons":0,"Writers":0,"Readers":0,"Consumers":0,"Records":0}}]}`

// ubGoldenDigest is ubGolden's brief digest computed outside Go, by the
// recipe the delivery-record format publishes: sha256 over the bytes after
// the first `,"lines":` up to the closing brace, first 12 hex.
const ubGoldenDigest = "e1f17930f38e"

// TestUnitBrief_V1FileBytes (W-M2.3 hook half; J2 point 5, ruling
// 03:58 (2)): the v1 file's bytes and its brief digest are fixed. Break:
// tag a fact field, rename one, or change the lines key, and this fails
// before any reader meets a file it cannot read.
func TestUnitBrief_V1FileBytes(t *testing.T) {
	data, err := encodeUnitBrief(ubBrief(t), "g:r1#unit:py:store", "g:r1#file:brief/brief.py", graph.Algorithmic, "prompt-7")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ubGolden {
		t.Errorf("the v1 file's bytes moved:\n got %s\nwant %s", data, ubGolden)
	}
	if d, err := unitBriefDigest(data); err != nil || d != ubGoldenDigest {
		t.Errorf("digest %q (%v), want %q", d, err, ubGoldenDigest)
	}
}

// TestUnitBrief_DigestIgnoresThePrompt (ruling 03:58 (2)): the digest names
// the brief by its bound facts, so two prompts that write the same facts
// give two files and one digest, and a later rewrite of an identical brief
// is not a replacement. Witness: one bound fact changed (the lesson's
// title) changes the digest. A name past the consumer line's cap would not:
// bind keeps it only as a count, so it is not in the file.
func TestUnitBrief_DigestIgnoresThePrompt(t *testing.T) {
	enc := func(b engine.Brief, prompt string) ([]byte, string) {
		t.Helper()
		data, err := encodeUnitBrief(b, "g:r1#unit:py:store", "g:r1#file:brief/brief.py", graph.Algorithmic, prompt)
		if err != nil {
			t.Fatal(err)
		}
		d, err := unitBriefDigest(data)
		if err != nil {
			t.Fatal(err)
		}
		return data, d
	}
	f7, d7 := enc(ubBrief(t), "prompt-7")
	f8, d8 := enc(ubBrief(t), "prompt-8")
	if string(f7) == string(f8) {
		t.Fatal("the two prompts wrote one file: the prompt id is not in the header")
	}
	if d7 != d8 {
		t.Errorf("same facts, two digests: %s and %s", d7, d8)
	}
	other := engine.Rebind(ubRev, []engine.Line{
		{Key: "file:brief/brief.py", Slot: slot.NewCodeLine(slot.CodeFact{Path: "brief/brief.py", Role: slot.RoleConsumer,
			Names: []slot.Ident{"b1", "b2", "b3", "b4", "b5"}})},
		{Key: "dec:D1", Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeDecision, ID: "dec:D1",
			Path: "store/loader.py", Clause: "rows load in key order"})},
		{Key: "les:L1", Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: "les:L1",
			Title: "a reload lost rows", Source: "incidents/L1.md"})},
	})
	if _, d := enc(other, "prompt-7"); d == d7 {
		t.Errorf("witness: a changed name kept the digest %s", d)
	}
}

// TestUnitBrief_DigestRefusesBytesWithNoLines: bytes with no lines value
// after another key, or not ending on the object's brace (a trailing
// newline included), name no brief. Witness: the closed form digests.
func TestUnitBrief_DigestRefusesBytesWithNoLines(t *testing.T) {
	const closed = `{"v":1,"lines":[]}`
	if _, err := unitBriefDigest([]byte(closed)); err != nil {
		t.Fatalf("witness: %q names no brief: %v", closed, err)
	}
	for _, data := range []string{"", "{}", `{"v":1,"line":[]}`, `{"lines":[]}`, closed + "\n"} {
		if d, err := unitBriefDigest([]byte(data)); err == nil {
			t.Errorf("%q: digest %q, want an error", data, d)
		}
	}
}
