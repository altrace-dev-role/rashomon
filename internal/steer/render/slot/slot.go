// Package slot is the renderer's second validation layer: the types a
// pattern's binding struct is made of (tech 4.2.7: "Each pattern binds its
// slots in a struct of `internal/steer/render/slot` types only"), each with
// the charset, range and semantic checks of archive 40.3.8's slot table.
// render.New runs every check before a value reaches a message and refuses
// the message when one fails, so a secret-shaped value is refused here and
// never rendered (STR-13). A check's error names the slot and the rule,
// never the value, because the value may be the secret it refused.
//
// Every regular expression in this package is compiled lazily, on first
// use, inside a sync.Once -- never at package scope -- per RES-2.
package slot

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Value is a slot value: one of this package's types and nothing else. The
// unexported method seals the set, so a binding struct cannot carry a type
// with a check of its own choosing.
type Value interface {
	// Validate reports why the value may not render as it is, or nil.
	Validate() error
	// Text is the value as a template renders it; it is meaningful only
	// for a value whose Validate returned nil.
	Text() string
	// bind returns the value as a message holds it once checked.
	bind() (Value, error)
}

// ErrNilSlot is returned by Bind for a nil Value.
var ErrNilSlot = errors.New("slot: nil value")

// Bind checks v and returns it as a rendered message holds it: v itself for
// every type but PathList, whose secret-shaped paths are left out rather
// than refusing the message (tech 4.2.7). It returns v's own error when v
// fails its check.
func Bind(v Value) (Value, error) {
	if v == nil {
		return nil, ErrNilSlot
	}
	return v.bind()
}

// checked returns v when err is nil, so each type's bind is one line.
func checked(v Value, err error) (Value, error) {
	if err != nil {
		return nil, err
	}
	return v, nil
}

// lazyRegexp compiles pattern on first use and never again. RES-2 forbids
// a package-scope regexp.MustCompile; this is the on-first-use form the
// rule asks for instead, so a slot type whose Validate is never called
// never pays for a regexp compile it did not need.
type lazyRegexp struct {
	once    sync.Once
	re      *regexp.Regexp
	pattern string
}

func (l *lazyRegexp) get() *regexp.Regexp {
	l.once.Do(func() {
		l.re = regexp.MustCompile(l.pattern)
	})
	return l.re
}

// charset checks s against re's pattern.
func charset(slot, s string, re *lazyRegexp) error {
	if !re.get().MatchString(s) {
		return fmt.Errorf("slot: %s does not match %s", slot, re.pattern)
	}
	return nil
}

// inRange checks lo <= n <= hi.
func inRange(slot string, n, lo, hi int) error {
	if n < lo || n > hi {
		return fmt.Errorf("slot: %s is outside %d..%d", slot, lo, hi)
	}
	return nil
}

// quoted wraps a value a repository can name in single quotes, so a template
// can render it only inside a quote (tech 4.2.7).
func quoted(s string) string { return "'" + s + "'" }

// Program is a program name (archive 40.3.8: `^[A-Za-z0-9._+-]{1,64}$`,
// "from `shape` only"). A repository can name one, so it renders inside a
// quote (tech 4.2.7: "`program 'aws'`"). It passes the secret deny list and
// the entropy test after its charset (screened); Gate's words do too.
type Program string

var programRE = &lazyRegexp{pattern: `^[A-Za-z0-9._+-]{1,64}$`}

func (p Program) Validate() error {
	if err := charset("program", string(p), programRE); err != nil {
		return err
	}
	return screened("program", string(p))
}

func (p Program) Text() string         { return quoted(string(p)) }
func (p Program) bind() (Value, error) { return checked(p, p.Validate()) }

// Gate is a gate's program and, optionally, its subcommand (archive 40.3.8:
// "one or two `Program`-charset words, at most 40 bytes (e.g. `go test`,
// `npm test`, `make test`)"; "never arguments"). A word count alone does not
// keep arguments out, so a word that reads as a flag (a leading '-') or is
// made only of dots is refused too.
type Gate string

const maxGateBytes = 40

func (g Gate) Validate() error {
	s := string(g)
	if len(s) > maxGateBytes {
		return fmt.Errorf("slot: gate is over %d bytes", maxGateBytes)
	}
	words := strings.Split(s, " ")
	if len(words) > 2 {
		return errors.New("slot: gate has more than two words")
	}
	for _, w := range words {
		if err := Program(w).Validate(); err != nil {
			return fmt.Errorf("slot: gate word: %w", err)
		}
		if strings.HasPrefix(w, "-") || strings.Trim(w, ".") == "" {
			return errors.New("slot: gate has an argument-shaped word")
		}
	}
	return nil
}

// Text quotes the gate: its first word is a program, which a repository can
// name (tech 4.2.7), so it renders only inside a quote, after a word that
// names it ("gate 'go test'"), like Program (P07N-11).
func (g Gate) Text() string         { return quoted(string(g)) }
func (g Gate) bind() (Value, error) { return checked(g, g.Validate()) }

// factRE is the charset FactValue and Mechanism share (archive 40.3.8:
// `^[A-Za-z0-9._:/@+=-]{1,128}$`).
var factRE = &lazyRegexp{pattern: `^[A-Za-z0-9._:/@+=-]{1,128}$`}

// Mechanism names the mechanism a note cites -- a profile, a role, an env
// var name, a vault path -- never its value (archive 40.3.8: the FactValue
// charset, "no spaces: an identifier"; "from a spec or approved fact only;
// same deny list"). It runs the entropy test FactValue runs as well: a
// spec can name any value here, whatever its trust.
type Mechanism string

func (m Mechanism) Validate() error {
	s := string(m)
	if err := charset("mechanism", s, factRE); err != nil {
		return err
	}
	return screened("mechanism", s)
}

func (m Mechanism) Text() string         { return string(m) }
func (m Mechanism) bind() (Value, error) { return checked(m, m.Validate()) }

// LessonID is a lesson's id (LEARN-DELIVER1, L7): learn's id form, which
// is verify.SlotID's charset, `^[a-z0-9][a-z0-9._:@-]{0,63}$`. A person
// approved the lesson, but its id came with the row (an import file names
// it), so it runs Mechanism's screens. An identifier, it renders unquoted
// after the word "lesson".
type LessonID string

var lessonIDRE = &lazyRegexp{pattern: `^[a-z0-9][a-z0-9._:@-]{0,63}$`}

func (l LessonID) Validate() error {
	if err := charset("lesson id", string(l), lessonIDRE); err != nil {
		return err
	}
	return screened("lesson id", string(l))
}

func (l LessonID) Text() string         { return string(l) }
func (l LessonID) bind() (Value, error) { return checked(l, l.Validate()) }

// Approved is text from a trusted spec's `approved` or `alternative` field
// (archive 40.3.8): its charset, no URL scheme, and none of four words as
// ASCII case-insensitive whole words. It renders only inside an attributed
// quote, which is what exempts it from the whole-string banned-word rule.
type Approved string

var approvedRE = &lazyRegexp{pattern: `^[A-Za-z0-9 ,.;:()_/+='-]{1,160}$`}

// urlSchemeRE finds a URL scheme with or without "//": any scheme before
// "://", or a known scheme before ':' and a non-space ("javascript:x",
// "data:text/html", "file:/etc"). A bare "word: value" is not a scheme, so
// "region: us-east-1" passes.
var urlSchemeRE = &lazyRegexp{pattern: `[A-Za-z][A-Za-z0-9+.-]*://|(?i:(?:^|[^A-Za-z0-9+.-])(?:https?|ftps?|sftp|file|data|javascript|vbscript|mailto|ssh|git|s3|gs|wss?|tel|sms|blob|about|ldap|smb|telnet|urn):[^ ])`}

// approvedBanned are the words Approved refuses (archive 40.3.8).
var approvedBanned = []string{"ignore", "instruction", "system", "rashomon"}

// Validate runs the slot table's checks (charset, no URL scheme, the four
// banned words), refuses a quote that is not an apostrophe between two
// letters or digits, because Approved renders inside a quote that its own
// quote would close or shift (P07N-9), and runs the secret deny list and
// the entropy test over each word, the check VERIFY applies to the same
// `alternative` field (archive 20.2.8, TestVER47), so a token never renders
// into model text before P-13's masker exists (P07N-10; FD-2).
func (a Approved) Validate() error {
	s := string(a)
	if err := charset("approved text", s, approvedRE); err != nil {
		return err
	}
	if urlSchemeRE.get().MatchString(s) {
		return errors.New("slot: approved text holds a URL scheme")
	}
	if w, ok := firstWordOf(s, approvedBanned); ok {
		return fmt.Errorf("slot: approved text holds the banned word %q", w)
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' && (i == 0 || i == len(s)-1 || !isAlnum(s[i-1]) || !isAlnum(s[i+1])) {
			return errors.New("slot: approved text holds a quote that is not an apostrophe")
		}
	}
	if err := denied("approved text", s); err != nil {
		return err
	}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" ,;()", r) }) {
		if highEntropy(w) {
			return fmt.Errorf("%w: approved text holds a word that fails the entropy test", ErrSecretShaped)
		}
	}
	return nil
}

func (a Approved) Text() string         { return quoted(string(a)) }
func (a Approved) bind() (Value, error) { return checked(a, a.Validate()) }

// firstWordOf returns the first whole word of s, a maximal run of ASCII
// letters and digits, that equals one of words, ASCII case-insensitively.
func firstWordOf(s string, words []string) (string, bool) {
	start := -1
	for i := 0; i <= len(s); i++ {
		if i < len(s) && isAlnum(s[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			for _, w := range words {
				if strings.EqualFold(s[start:i], w) {
					return w, true
				}
			}
			start = -1
		}
	}
	return "", false
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// Count is a count (archive 40.3.8: 0..1,000,000).
type Count int

const maxCount = 1_000_000

func (c Count) Validate() error      { return inRange("count", int(c), 0, maxCount) }
func (c Count) Text() string         { return strconv.Itoa(int(c)) }
func (c Count) bind() (Value, error) { return checked(c, c.Validate()) }

// Exit is a process exit status (archive 40.3.8: -255..255).
type Exit int

const maxExit = 255

func (e Exit) Validate() error      { return inRange("exit code", int(e), -maxExit, maxExit) }
func (e Exit) Text() string         { return strconv.Itoa(int(e)) }
func (e Exit) bind() (Value, error) { return checked(e, e.Validate()) }

// Minutes is a span in minutes (archive 40.3.8: 0..10,080, one week).
type Minutes int

const maxMinutes = 10_080

func (m Minutes) Validate() error      { return inRange("minutes", int(m), 0, maxMinutes) }
func (m Minutes) Text() string         { return strconv.Itoa(int(m)) }
func (m Minutes) bind() (Value, error) { return checked(m, m.Validate()) }

// TaskID names a task (archive 40.3.8: `^[A-Za-z0-9._-]{1,64}$`).
type TaskID string

var taskIDRE = &lazyRegexp{pattern: `^[A-Za-z0-9._-]{1,64}$`}

func (t TaskID) Validate() error {
	if err := charset("task id", string(t), taskIDRE); err != nil {
		return err
	}
	return screened("task id", string(t))
}

func (t TaskID) Text() string         { return string(t) }
func (t TaskID) bind() (Value, error) { return checked(t, t.Validate()) }

// AgentType is a subagent type (archive 40.3.8: `^[A-Za-z0-9:._-]{1,64}$`),
// which admits Claude Code's own (`general-purpose`, `Explore`) and plugin
// agents (`plugin:agent`). A repository can name one, so it renders inside
// a quote (tech 4.2.7: "`agent type 'reviewer'`").
type AgentType string

var agentTypeRE = &lazyRegexp{pattern: `^[A-Za-z0-9:._-]{1,64}$`}

func (a AgentType) Validate() error {
	if err := charset("agent type", string(a), agentTypeRE); err != nil {
		return err
	}
	return screened("agent type", string(a))
}

func (a AgentType) Text() string         { return quoted(string(a)) }
func (a AgentType) bind() (Value, error) { return checked(a, a.Validate()) }

// RuleID names a rule of a loaded spec (archive 40.3.8:
// `^[a-z0-9][a-z0-9-]{0,47}$`, "from a loaded spec").
type RuleID string

var ruleIDRE = &lazyRegexp{pattern: `^[a-z0-9][a-z0-9-]{0,47}$`}

func (r RuleID) Validate() error {
	if err := charset("rule id", string(r), ruleIDRE); err != nil {
		return err
	}
	return screened("rule id", string(r))
}

func (r RuleID) Text() string         { return string(r) }
func (r RuleID) bind() (Value, error) { return checked(r, r.Validate()) }

// Model is a model id from the payload (archive 40.3.8:
// `^[A-Za-z0-9._\[\]-]{1,64}$`), which admits bracketed ids such as
// `claude-opus-4-6[1m]`.
type Model string

var modelRE = &lazyRegexp{pattern: `^[A-Za-z0-9._\[\]-]{1,64}$`}

func (m Model) Validate() error {
	if err := charset("model", string(m), modelRE); err != nil {
		return err
	}
	return screened("model", string(m))
}

func (m Model) Text() string         { return string(m) }
func (m Model) bind() (Value, error) { return checked(m, m.Validate()) }

// USD is a dollar amount, rendered with two decimals (archive 40.3.8: "`%.2f`
// of 0..10,000"); the template supplies the "$". NaN, an infinity and a
// negative zero are refused, the last because it would render "-0.00".
type USD float64

const maxUSD = 10_000

func (u USD) Validate() error {
	v := float64(u)
	if math.IsNaN(v) || math.Signbit(v) || v > maxUSD {
		return fmt.Errorf("slot: USD amount is outside 0..%d", maxUSD)
	}
	return nil
}

func (u USD) Text() string         { return strconv.FormatFloat(float64(u), 'f', 2, 64) }
func (u USD) bind() (Value, error) { return checked(u, u.Validate()) }

// Date is a calendar date, YYYY-MM-DD (archive 40.3.8). time.Parse refuses a
// month or day out of range ("2026-02-30": day out of range), so a value of
// the right shape that parses names a real date.
type Date string

var dateRE = &lazyRegexp{pattern: `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`}

func (d Date) Validate() error {
	s := string(d)
	if err := charset("date", s, dateRE); err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return errors.New("slot: date is not a calendar date")
	}
	return nil
}

func (d Date) Text() string         { return string(d) }
func (d Date) bind() (Value, error) { return checked(d, d.Validate()) }
