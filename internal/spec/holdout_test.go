package spec

import (
	"slices"
	"strconv"
	"testing"
	"time"
)

// withHoldout is a minimal spec with body as its holdout object.
func withHoldout(body string) string { return top(`"holdout": ` + body) }

// parsed is Parse's File for in, failing the test when in is refused.
func parsed(t *testing.T, in string) *File {
	t.Helper()
	f, _, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse(%s): %v", in, err)
	}
	return f
}

func day(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func wantOn(t *testing.T, name string, h Holdout, now time.Time, on bool, rate int) {
	t.Helper()
	if gotOn, gotRate := h.On(now); gotOn != on || gotRate != rate {
		t.Errorf("%s: On(%s) = %t, %d; want %t, %d", name, now.Format(time.RFC3339), gotOn, gotRate, on, rate)
	}
}

// TestMS5a_NoHoldoutObjectIsOnAtDefaultRate: a file without the object, and an
// empty object, are on at 1000 basis points; a stated rate is the rate.
func TestMS5a_NoHoldoutObjectIsOnAtDefaultRate(t *testing.T) {
	now := day("2026-10-29T12:00:00Z")
	none := parsed(t, top(""))
	if none.Holdout != (Holdout{}) {
		t.Errorf("no holdout object read as %+v, want the zero Holdout", none.Holdout)
	}
	wantOn(t, "no object", none.Holdout, now, true, DefaultHoldoutBP)
	empty := parsed(t, withHoldout(`{}`))
	if empty.Holdout.RateBP != DefaultHoldoutBP {
		t.Errorf("empty object rate_bp %d, want %d", empty.Holdout.RateBP, DefaultHoldoutBP)
	}
	wantOn(t, "empty object", empty.Holdout, now, true, DefaultHoldoutBP)
	wantOn(t, "rate 1500", parsed(t, withHoldout(`{"rate_bp": 1500}`)).Holdout, now, true, 1500)
}

// TestMS5b_OffIsOff: off true turns the holdout off at any time; off false is
// the default.
func TestMS5b_OffIsOff(t *testing.T) {
	now := day("2026-10-29T12:00:00Z")
	wantOn(t, "off true", parsed(t, withHoldout(`{"off": true}`)).Holdout, now, false, 0)
	wantOn(t, "off true with a rate", parsed(t, withHoldout(`{"off": true, "rate_bp": 2000}`)).Holdout, now, false, 0)
	wantOn(t, "off false", parsed(t, withHoldout(`{"off": false}`)).Holdout, now, true, DefaultHoldoutBP)
	runRows(t, []row{bad("off as a string", withHoldout(`{"off": "true"}`), ReasonType, "/holdout/off")})
}

// TestMS5c_RateBounds: 500 and 2000 are accepted, 499 and 2001 refused.
func TestMS5c_RateBounds(t *testing.T) {
	runRows(t, []row{
		ok("rate 500", withHoldout(`{"rate_bp": 500}`)),
		ok("rate 2000", withHoldout(`{"rate_bp": 2000}`)),
		bad("rate 499", withHoldout(`{"rate_bp": 499}`), ReasonHoldoutRate, "/holdout/rate_bp"),
		bad("rate 2001", withHoldout(`{"rate_bp": 2001}`), ReasonHoldoutRate, "/holdout/rate_bp"),
		bad("rate 0", withHoldout(`{"rate_bp": 0}`), ReasonHoldoutRate, "/holdout/rate_bp"),
		bad("rate as a string", withHoldout(`{"rate_bp": "1000"}`), ReasonType, "/holdout/rate_bp"),
		bad("rate with a fraction", withHoldout(`{"rate_bp": 1000.5}`), ReasonNotInteger, "/holdout/rate_bp"),
	})
	for _, rate := range []int{500, 2000} {
		in := withHoldout(`{"rate_bp": ` + strconv.Itoa(rate) + `}`)
		if got := parsed(t, in).Holdout.RateBP; got != rate {
			t.Errorf("rate_bp %d read as %d", rate, got)
		}
	}
}

// TestMS5d_WindowClosesTheDayAfter: started 2026-10-27 with a 30-day window is
// on through 2026-11-26 (UTC) and off from 2026-11-27; window_days is 1 to 365.
func TestMS5d_WindowClosesTheDayAfter(t *testing.T) {
	h := parsed(t, withHoldout(`{"started": "2026-10-27", "window_days": 30}`)).Holdout
	if !h.Started.Equal(day("2026-10-27T00:00:00Z")) || h.WindowDays != 30 {
		t.Fatalf("window read as %s and %d days", h.Started, h.WindowDays)
	}
	wantOn(t, "the start day", h, day("2026-10-27T00:00:00Z"), true, DefaultHoldoutBP)
	wantOn(t, "the closing day's first second", h, day("2026-11-26T00:00:00Z"), true, DefaultHoldoutBP)
	wantOn(t, "the closing day's last second", h, day("2026-11-26T23:59:59Z"), true, DefaultHoldoutBP)
	wantOn(t, "the day after", h, day("2026-11-27T00:00:00Z"), false, 0)
	wantOn(t, "a year after", h, day("2027-11-27T00:00:00Z"), false, 0)
	wantOn(t, "before the start", h, day("2026-10-01T00:00:00Z"), true, DefaultHoldoutBP)
	one := parsed(t, withHoldout(`{"started": "2026-10-27", "window_days": 1, "rate_bp": 700}`)).Holdout
	wantOn(t, "one day, its last day", one, day("2026-10-28T23:59:59Z"), true, 700)
	wantOn(t, "one day, the day after", one, day("2026-10-29T00:00:00Z"), false, 0)
	runRows(t, []row{
		ok("window 1", withHoldout(`{"started": "2026-10-27", "window_days": 1}`)),
		ok("window 365", withHoldout(`{"started": "2026-10-27", "window_days": 365}`)),
		bad("window 0", withHoldout(`{"started": "2026-10-27", "window_days": 0}`), ReasonHoldoutWindow, "/holdout/window_days"),
		bad("window 366", withHoldout(`{"started": "2026-10-27", "window_days": 366}`), ReasonHoldoutWindow, "/holdout/window_days"),
		bad("window as a string", withHoldout(`{"started": "2026-10-27", "window_days": "30"}`), ReasonType, "/holdout/window_days"),
		bad("started not a day of its month", withHoldout(`{"started": "2026-02-30", "window_days": 30}`), ReasonHoldoutWindow, "/holdout/started"),
		bad("started without leading zeros", withHoldout(`{"started": "2026-1-5", "window_days": 30}`), ReasonHoldoutWindow, "/holdout/started"),
		bad("started with a time", withHoldout(`{"started": "2026-10-27T00:00:00Z", "window_days": 30}`), ReasonHoldoutWindow, "/holdout/started"),
		bad("started empty", withHoldout(`{"started": "", "window_days": 30}`), ReasonEmpty, "/holdout/started"),
	})
}

// TestMS5e_StartedWithoutWindowRefused: started and window_days come together
// or not at all; the pointer names the one missing.
func TestMS5e_StartedWithoutWindowRefused(t *testing.T) {
	runRows(t, []row{
		bad("started alone", withHoldout(`{"started": "2026-10-27"}`), ReasonHoldoutWindow, "/holdout/window_days"),
		bad("window_days alone", withHoldout(`{"window_days": 30}`), ReasonHoldoutWindow, "/holdout/started"),
		ok("both", withHoldout(`{"started": "2026-10-27", "window_days": 30}`)),
		ok("neither", withHoldout(`{"off": false}`)),
	})
}

// TestMS5f_UnknownKeyUnderHoldoutRefused: the object's keys are closed
// (parse.go's object, ReasonUnknownKey), and it is an object.
func TestMS5f_UnknownKeyUnderHoldoutRefused(t *testing.T) {
	runRows(t, []row{
		bad("unknown key", withHoldout(`{"off": false, "of": true}`), ReasonUnknownKey, "/holdout/of"),
		bad("a key in another case", withHoldout(`{"Off": true}`), ReasonUnknownKey, "/holdout/Off"),
		bad("not an object", withHoldout(`true`), ReasonType, "/holdout"),
		ok("every key", withHoldout(`{"off": false, "rate_bp": 1200, "started": "2026-10-27", "window_days": 30}`)),
	})
}

// TestMS5g_DigestMovesWithHoldout: the digest covers the holdout object; two
// files differing only in off differ, and a key order does not.
func TestMS5g_DigestMovesWithHoldout(t *testing.T) {
	on, off := digestOf(t, withHoldout(`{"off": false}`)), digestOf(t, withHoldout(`{"off": true}`))
	if on == off {
		t.Errorf("off false and off true share digest %s", on)
	}
	if none := digestOf(t, top("")); none == on {
		t.Errorf("a file with a holdout object shares the digest of one without: %s", none)
	}
	a := digestOf(t, withHoldout(`{"rate_bp": 700, "off": true}`))
	if b := digestOf(t, withHoldout(`{"off": true, "rate_bp": 700}`)); a != b {
		t.Errorf("key order moved the digest: %s and %s", a, b)
	}
}

// TestSP14_SuppliesIsClosed: a gate's supplies is drawn from supplyKinds, each
// kind once.
func TestSP14_SuppliesIsClosed(t *testing.T) {
	all := withGate(kv{"supplies", `["real_data_run", "hostile_input_run", "cross_check"]`})
	runRows(t, []row{
		ok("the three kinds", all),
		ok("one kind", withGate(kv{"supplies", `["cross_check"]`})),
		bad("a fourth kind", withGate(kv{"supplies", `["real_data_run", "test_note"]`}), ReasonEnum, "/gates/0/supplies/1"),
		bad("a kind twice", withGate(kv{"supplies", `["cross_check", "real_data_run", "cross_check"]`}), ReasonDuplicateID, "/gates/0/supplies/2"),
		bad("empty", withGate(kv{"supplies", `[]`}), ReasonEmpty, "/gates/0/supplies"),
		bad("not a list", withGate(kv{"supplies", `"cross_check"`}), ReasonType, "/gates/0/supplies"),
	})
	want := []string{"real_data_run", "hostile_input_run", "cross_check"}
	if got := parsed(t, all).Gates[0].Supplies; !slices.Equal(got, want) {
		t.Errorf("supplies read as %v, want %v", got, want)
	}
	if got := parsed(t, withGate()).Gates[0].Supplies; got != nil {
		t.Errorf("a gate without supplies read %v, want nil", got)
	}
}

// TestSP14b_DigestMovesWithSupplies: two files differing only in a gate's
// supplies have different digests.
func TestSP14b_DigestMovesWithSupplies(t *testing.T) {
	a := digestOf(t, withGate(kv{"supplies", `["cross_check"]`}))
	b := digestOf(t, withGate(kv{"supplies", `["real_data_run"]`}))
	if a == b {
		t.Errorf("different supplies share digest %s", a)
	}
}
