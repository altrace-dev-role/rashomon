package secretshape

import (
	"math"
	"strings"
)

// denyRE is the secret deny list (archive 40.3.8: "gitleaks/trufflehog rule
// set: AWS key ids, 40-character keys, `user:token@`, bearer tokens,
// `PGPASSWORD=`"): the masker's token formats R12-R20 (archive 10.2.4), the
// formats R20 names by prefix only widened to a prefix and a long token,
// URL user-info (R5), a bearer token (R7), and a secret-named key followed by
// a value (R1, R21); and gitleaks' Mailgun, Twilio, Postman and private-key
// rules (P07N-7). The 40-character key is hasKeyRun's, as RE2 has no
// lookaround to bound a run. Over-matching costs a note or a listed path;
// under-matching would print a secret, so each rule errs wide, but the
// prefixed formats start at a word's start, as the masker's R20 does with
// \b, and the three whose prefix is also a common word part (hf_, SG.,
// dapi) take gitleaks' token body, so an ordinary file name such as
// rapidapi_client.py or MSG.handler.go is kept; and a bearer token needs a
// separator, so BearerTokenAuthenticationFilterTests.java is too (P07N-12).
var denyRE = &lazyRegexp{pattern: strings.Join([]string{
	`(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}`,
	`gh[pousr]_[A-Za-z0-9]{36,}`,
	`github_pat_[A-Za-z0-9_]{22,}`,
	`glpat-[A-Za-z0-9_-]{20,}`,
	`xox[abposr]-[A-Za-z0-9-]{10,}`,
	`sk-ant-[A-Za-z0-9_-]{20,}`,
	`sk-(?:proj-)?[A-Za-z0-9_-]{20,}`,
	`(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}`,
	`AIza[0-9A-Za-z_-]{35}`,
	`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`,
	`npm_[A-Za-z0-9]{36}`,
	`pypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}`,
	`hvs\.[A-Za-z0-9_-]{24,}`,
	`(?:^|[^A-Za-z0-9])(?:dop_v1_|shp(?:at|ca|pa|ss)_|ATATT|xapp-|dp\.st\.)[A-Za-z0-9_.=-]{16,}`,
	`(?:^|[^A-Za-z0-9])(?:hf_[A-Za-z]{34}|SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}|dapi[a-f0-9]{32})`,
	`(?:^|[^A-Za-z0-9])(?:key-[0-9a-z]{32}|SK[0-9a-fA-F]{32}|PMAK-[a-f0-9]{24}-[a-f0-9]{34})`,
	`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY`,
	`[A-Za-z0-9._~%+-]+:[^@/\s]+@`,
	`(?:^|[^A-Za-z0-9])(?i:bearer)[\s:=]+[A-Za-z0-9._~+/=-]{20,}`,
	`(?i:(?:pgpassword|mysql_pwd|database_url|connection_string|client_secret|aws_secret_access_key|aws_session_token|_authtoken|[a-z0-9_]*(?:password|passwd|_pass|_pwd|secret|token|api_key|apikey|access_key|secret_key|private_key|credentials?|_auth|session_key|cookie|_pat|_dsn|_key|passphrase|accountkey))\s*[:=])`,
}, "|")}

// hasKeyRun reports a 40-character key: a maximal run of exactly 40
// characters of the base64 alphabet holding an upper-case letter, a
// lower-case letter and a digit, the shape of an AWS secret access key,
// which has no prefix. A lower-case 40-hex digest is not one.
func hasKeyRun(s string) bool {
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && (isAlnum(s[i]) || s[i] == '/' || s[i] == '+') {
			continue
		}
		if i-start == 40 && mixedClasses(s[start:i]) {
			return true
		}
		start = i + 1
	}
	return false
}

func mixedClasses(s string) bool {
	var upper, lower, digit bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		upper = upper || c >= 'A' && c <= 'Z'
		lower = lower || c >= 'a' && c <= 'z'
		digit = digit || c >= '0' && c <= '9'
	}
	return upper && lower && digit
}

// entropyAlphabets are the entropy test's alphabet sizes, smallest first
// (tech 1.2.4: "16, 36, 62, else 64").
var entropyAlphabets = [4]int{16, 36, 62, 64}

const (
	entropyMinRun = 20  // a shorter run is never secret-shaped
	entropyMaxRow = 128 // a longer run uses this row of the table
)

// highEntropy is the entropy test (tech 1.2.4, archive 10.2.4: one
// definition, `mask.HighEntropy`, for every entropy check): s is split at
// '.', ':' and '@'; a run of n >= 20 characters is high-entropy when its
// Shannon entropy is at least mu(k, n) - 3 sigma(k, n), with k the smallest
// alphabet holding the run and mu, sigma read from entropyTable (a longer
// run uses the n = 128 row); a run of exactly 40 or 64 hex characters
// always is. The spec gives the shared definition to internal/mask (P-13),
// which main does not have; this package is that one definition now, pinned
// to REC-7's canaries by TestEntropyTestFlagsTheCanaries.
func highEntropy(s string) bool {
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' || s[i] == ':' || s[i] == '@' {
			if runHighEntropy(s[start:i]) || tokenInRun(s[start:i]) {
				return true
			}
			start = i + 1
		}
	}
	return joinedRunHighEntropy(s)
}

// minJoinPart is the shortest part joinedRunHighEntropy joins: longer than
// every group of a UUID but its last, so a UUID is never joined.
const minJoinPart = 12

// joinable reports a part joinedRunHighEntropy may join: at least
// minJoinPart characters holding a digit or both letter cases, so a random
// part almost always qualifies and a lower-case word never does (1.2.4's
// threshold already refuses some single English words of 20 letters or
// more; joining "springframework" to "autoconfigure" would refuse more).
func joinable(part string) bool {
	if len(part) < minJoinPart {
		return false
	}
	var upper, lower bool
	for i := 0; i < len(part); i++ {
		c := part[i]
		if c >= '0' && c <= '9' {
			return true
		}
		upper = upper || c >= 'A' && c <= 'Z'
		lower = lower || c >= 'a' && c <= 'z'
	}
	return upper && lower
}

// tokenSeparators are the bytes the entropy checks split a value at:
// 1.2.4's '.', ':' and '@', and tokenInRun's '-', '_', '+', '/' and '='.
const tokenSeparators = ".:@-_+/="

// joinedRunHighEntropy is the slot layer's second addition to tech 1.2.4,
// a P-07 proposal for P-13 beside tokenInRun (P07N-7): a token cut by
// separators into runs each under 20 characters (hex16-hex16, two base62
// halves joined by '.') is never tested by 1.2.4 or by tokenInRun. So
// every maximal sequence of two or more consecutive joinable parts is
// tested as one run, the separators removed. A
// UUID (8-4-4-4-12) has no two such parts in a row and is kept, so a
// UUID-shaped key still passes; refusing those is the owner's call (P-13,
// tech 1.2.4), since a UUID is more often an identifier. A joined run that
// reads as camelCase words (identifierShaped: "OAuth2Client" joined to
// "refreshTokenExpiry") is a name, not a token, and is kept.
func joinedRunHighEntropy(s string) bool {
	var joined []byte
	parts := 0
	flush := func() bool {
		hit := parts >= 2 && runHighEntropy(string(joined)) && !identifierShaped(string(joined))
		joined, parts = joined[:0], 0
		return hit
	}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(tokenSeparators, r) }) {
		if !joinable(part) {
			if flush() {
				return true
			}
			continue
		}
		joined = append(joined, part...)
		parts++
	}
	return flush()
}

// maxTokenPrefix is the longest letter prefix tokenInRun strips.
const maxTokenPrefix = 8

// tokenInRun is the slot layer's addition to tech 1.2.4's entropy test, a
// P-07 proposal for the owner of mask.HighEntropy (P-13), which shares the
// definition (P07N-7): 1.2.4 picks k from every character of the run, so
// one '-', '_' or capital letter beside a hex token moves it to the 62 or 64
// column, whose threshold a 16-symbol random string cannot reach. So each
// part of the run between '-', '_', '+', '/' and '=' is tested on its own,
// and so is each part without a leading run of up to maxTokenPrefix
// letters when that leaves a smaller alphabet ("SK" before 32 hex digits).
// A part under 20 characters is not tested here, as 1.2.4 has it;
// joinedRunHighEntropy tests consecutive long parts joined. A part that
// reads as camelCase words ("UserServiceImpl2Test") is kept, as 1.2.4 keeps
// it inside its whole run. A stripped rest needs no such exemption: its
// alphabet is smaller than its part's, so it holds one letter case and is
// never identifierShaped.
func tokenInRun(run string) bool {
	parts := strings.FieldsFunc(run, func(r rune) bool { return strings.ContainsRune("-_+/=", r) })
	for _, part := range parts {
		if part != run && runHighEntropy(part) && !identifierShaped(part) {
			return true
		}
		k := alphabetIndex(part)
		for p := 1; p <= maxTokenPrefix && p < len(part) && isLetter(part[p-1]); p++ {
			rest := part[p:]
			if len(rest) >= entropyMinRun && alphabetIndex(rest) < k && runHighEntropy(rest) {
				return true
			}
		}
	}
	return false
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func runHighEntropy(run string) bool {
	n := len(run)
	if n < entropyMinRun {
		return false
	}
	if (n == 40 || n == 64) && isHex(run) {
		return true
	}
	m := entropyTable[min(n, entropyMaxRow)-entropyMinRun][alphabetIndex(run)]
	return shannon(run) >= m.mu-3*m.sigma
}

// shannon is s's Shannon entropy in bits per character.
func shannon(s string) float64 {
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	n := float64(len(s))
	h := 0.0
	for _, c := range counts {
		if c > 0 {
			p := float64(c) / n
			h -= p * math.Log2(p)
		}
	}
	return h
}

// alphabetIndex returns the index into entropyAlphabets of the smallest
// alphabet holding s: hex digits of one case, digits and letters of one
// case, digits and letters, anything else.
func alphabetIndex(s string) int {
	lowerHex, upperHex, lower36, upper36, base62 := true, true, true, true, true
	for i := 0; i < len(s); i++ {
		c := s[i]
		digit := c >= '0' && c <= '9'
		lower := c >= 'a' && c <= 'z'
		upper := c >= 'A' && c <= 'Z'
		lowerHex = lowerHex && (digit || c >= 'a' && c <= 'f')
		upperHex = upperHex && (digit || c >= 'A' && c <= 'F')
		lower36 = lower36 && (digit || lower)
		upper36 = upper36 && (digit || upper)
		base62 = base62 && (digit || lower || upper)
	}
	switch {
	case lowerHex || upperHex:
		return 0
	case lower36 || upper36:
		return 1
	case base62:
		return 2
	}
	return 3
}

// isHex reports a run of hex digits of either case.
func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// identifierShaped reports a run that reads as camelCase words, the shape
// of a name in a test file ("OAuth2Client", "refreshTokenExpiry",
// "UserServiceImpl2Test"): only letters and digits, an upper-case letter,
// at least three fifths lower-case letters, and every run of lower-case
// letters at least two long and begun at the start or straight after an
// upper-case letter. A random base62 run rarely is (0.27% at 20
// characters, the shortest either addition tests; TestIdentifierPartsAreKept
// bounds it under 0.5%), and a run with one letter case, as every hex, base36 and
// lower-case token has, never is. The names measured read 0.60
// (HTTP2ServerPushTests) to 0.89 lower-case.
func identifierShaped(s string) bool {
	upper, lower := false, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isAlnum(c) {
			return false
		}
		if c < 'a' || c > 'z' {
			upper = upper || c >= 'A' && c <= 'Z'
			continue
		}
		lower++
		if i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' {
			continue
		}
		startsWord := i == 0 || s[i-1] >= 'A' && s[i-1] <= 'Z'
		if !startsWord || i+1 == len(s) || s[i+1] < 'a' || s[i+1] > 'z' {
			return false
		}
	}
	return upper && 5*lower >= 3*len(s)
}

// isAlnum is the byte class hasKeyRun and identifierShaped share. On the
// steer branch it lived in slot.go beside the slot types, the one thing the
// lifted functions needed from outside them.
func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
