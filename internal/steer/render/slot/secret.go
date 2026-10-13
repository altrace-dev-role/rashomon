package slot

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// ErrSecretShaped is the error of a value refused by the secret deny list
// or the entropy test (STR-13).
var ErrSecretShaped = errors.New("slot: secret-shaped value")

// denyRE is the secret deny list (archive 40.3.8: "gitleaks/trufflehog rule
// set: AWS key ids, 40-character keys, `user:token@`, bearer tokens,
// `PGPASSWORD=`"): the masker's token formats R12-R20 (archive 10.2.4), the
// formats R20 names by prefix only widened to a prefix and a long token,
// URL user-info (R5), a bearer token (R7), and a secret-named key followed by
// a value (R1, R21). The 40-character key is hasKeyRun's, as RE2 has no
// lookaround to bound a run. Over-matching costs a note or a listed path;
// under-matching would print a secret, so each rule errs wide.
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
	`(?:hf_|SG\.|dop_v1_|shp(?:at|ca|pa|ss)_|dapi|ATATT|xapp-|dp\.st\.)[A-Za-z0-9_.=-]{16,}`,
	`[A-Za-z0-9._~%+-]+:[^@/\s]+@`,
	`(?i:bearer)[\s:=_+-]*[A-Za-z0-9._~+/=-]{20,}`,
	`(?i:(?:pgpassword|mysql_pwd|database_url|connection_string|client_secret|aws_secret_access_key|aws_session_token|_authtoken|[a-z0-9_]*(?:password|passwd|_pass|_pwd|secret|token|api_key|apikey|access_key|secret_key|private_key|credentials?|_auth|session_key|cookie|_pat|_dsn|_key|passphrase|accountkey))\s*[:=])`,
}, "|")}

// denied returns ErrSecretShaped, wrapped, when s matches the deny list.
func denied(slot, s string) error {
	if denyRE.get().MatchString(s) || hasKeyRun(s) {
		return fmt.Errorf("%w: %s matches the secret deny list", ErrSecretShaped, slot)
	}
	return nil
}

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
// always is. internal/mask, which owns the shared definition, is P-13's;
// this is the same rule, pinned to REC-7's canaries by
// TestSTR13_SecretShapedValuesRefused.
func highEntropy(s string) bool {
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' || s[i] == ':' || s[i] == '@' {
			if runHighEntropy(s[start:i]) {
				return true
			}
			start = i + 1
		}
	}
	return false
}

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
