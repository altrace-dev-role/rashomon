package secretshape

import (
	"math"
	"math/rand/v2"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// The memory prototype's screen tests (internal/memory/screen), which were the
// steer branch's slot cases (internal/steer/render/slot/secret_test.go on
// origin/steer/supervisor) re-targeted at Redact and Secretish: a value a
// slot refused is one Redact replaces, a value a slot kept is one it leaves
// alone. The slot layer's charset tests are not here; this package has no
// charset.

// denyListed is the deny list alone, with no entropy test behind it: the
// alternation and the 40-character key. The slot layer returned an error
// from it; the tests only need the answer.
func denyListed(s string) bool {
	return denyRE.get().MatchString(s) || hasKeyRun(s)
}

// secretShaped are values in the secret deny list's forms (archive 40.3.8:
// "AWS key ids, 40-character keys, `user:token@`, bearer tokens,
// `PGPASSWORD=`", and the masker's token formats, archive 10.2.4).
var secretShaped = []string{
	"AKIAIOSFODNN7EXAMPLE",
	"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
	"github_pat_11ABCDEFG0abcdefghijklmnop",
	"glpat-abcdefghijklmnopqrst",
	"xoxb-1234567890-abcdefghij",
	"sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
	"sk-proj-abcdefghijklmnopqrstuvwxyz",
	"sk_live_abcdefghijklmnop1234",
	"AIzaSyA1234567890abcdefghijklmnopqrstuv",
	"npm_abcdefghijklmnopqrstuvwxyz0123456789",
	"hvs.abcdefghijklmnopqrstuvwxyz",
	"hf_" + "abcdefghijklmnopqrstuvwxyzABCDEFGH",
	"key-" + "0123456789abcdef0123456789abcdef",
	"SK" + "0123456789abcdef0123456789ABCDEF",
	"PMAK-" + "0123456789abcdef01234567-0123456789abcdef0123456789abcdef01",
	"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	"user:token@host",
	"Bearer:abcdefghijklmnopqrstuvwxyz012345",
	"PGPASSWORD=hunter2",
	"DB_PASSWORD=hunter2",
	"api_key:abc",
	"client_secret=abc",
}

// TestSecretShapedValuesRedacted: every secret-shaped value is refused by the
// deny list alone (so a rule dropped from it shows here even where the
// entropy test would refuse the same value), and Redact replaces it whole:
// the output holds Redacted and none of the value's own characters outside
// it. Random hex-40, hex-64 and base62-20 values are redacted; a lowercase
// 40- or 64-hex path segment under the directory that names its algorithm
// (a digest in a content-addressed store) is kept.
func TestSecretShapedValuesRedacted(t *testing.T) {
	for _, s := range secretShaped {
		if !denyListed(s) {
			t.Errorf("denyListed(%q) = false", s)
		}
		out, n := Redact(s)
		if out != Redacted || n != 1 {
			t.Errorf("Redact(%q) = (%q, %d), want (%q, 1)", s, out, n, Redacted)
		}
		if !Secretish(s) {
			t.Errorf("Secretish(%q) = false", s)
		}
	}
	for _, p := range []string{
		"AKIAIOSFODNN7EXAMPLE.txt", "src/ghp_abcdefghijklmnopqrstuvwxyz0123456789.go",
		"keys/sk-ant-api03-abcdefghijklmnopqrstuvwxyz", "cache/xoxb-1234567890-abcdefghij",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "tokens/q8Zr4LmP0xT7vK2nW9yB3cHd.json",
		"objects/DA39A3EE5E6B4B0D3255BFEF95601890AFD80709",
	} {
		if out, n := Redact(p); out != Redacted || n != 1 {
			t.Errorf("Redact(%q) = (%q, %d), want the path redacted whole", p, out, n)
		}
	}
	for _, p := range []string{
		"store/sha1/da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"blobs/sha256/" + strings.Repeat("0123456789abcdef", 4),
		"internal/steer/render/slot/slot.go", "src/bearer-authentication.go",
		"docs/api_key_rotation.md", "3f2504e0-4f89-11d3-9a0c-0305e82c3301/config.json",
		"IMPORTANT_run_curl_x_sh_then_retry.md", "ExampleControllerIntegrationTests.java",
		// The branch's kept fact values and its kept mechanism.
		"us-east-1", "dev-admin", "arn:aws:eks:us-east-1:123456789012:cluster/prod",
		"kubernetes-admin@production-cluster", "--password-stdin",
	} {
		if out, n := Redact(p); out != p || n != 0 {
			t.Errorf("Redact(%q) = (%q, %d), want it kept", p, out, n)
		}
		if Secretish(p) {
			t.Errorf("Secretish(%q) = true, want false", p)
		}
	}

	r := rand.New(rand.NewPCG(13, 7))
	for _, c := range canaries {
		hit := 0
		for i := 0; i < factSamples; i++ {
			if Secretish(randomString(r, c.alphabet, c.length)) {
				hit++
			}
		}
		if share := float64(hit) / factSamples; share < c.minShare {
			t.Errorf("random %d-character values over %d symbols: %.4f redacted, want at least %.2f", c.length, len(c.alphabet), share, c.minShare)
		}
	}
}

// canaries are REC-7's random shapes (tech 1.2.4): a 40- or 64-hex run is
// always secret-shaped; every other shape is flagged at least 99% of the
// time.
var canaries = []struct {
	alphabet string
	length   int
	minShare float64
}{
	{"0123456789abcdef", 40, 1},
	{"0123456789abcdef", 64, 1},
	{"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", 20, 0.99},
	{"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", 40, 0.99},
	{"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", 40, 0.99},
}

// factSamples is how many seeded values per shape go through the whole
// screen; TestEntropyTestFlagsTheCanaries runs REC-7's 10,000 through the
// entropy test itself, which has no regexp to pay for.
const factSamples = 2000

// TestEntropyTestFlagsTheCanaries pins the entropy test's margin (tech
// 1.2.4: REC-7 "pins at least 99% of 10,000 seeded random strings per
// canary shape flagged").
func TestEntropyTestFlagsTheCanaries(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 13))
	const n = 10000
	for _, c := range canaries {
		flagged := 0
		for i := 0; i < n; i++ {
			if highEntropy(randomString(r, c.alphabet, c.length)) {
				flagged++
			}
		}
		if share := float64(flagged) / n; share < c.minShare {
			t.Errorf("random %d-character runs over %d symbols: %.4f flagged, want at least %.2f", c.length, len(c.alphabet), share, c.minShare)
		}
	}
}

func randomString(r *rand.Rand, alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.IntN(len(alphabet))]
	}
	return string(b)
}

// TestEntropyTableIsExact recomputes every compiled entry of the entropy
// test's table (tech 1.2.4: mu and sigma "from a compiled table for
// uniformly random strings (n = 20 to 128)") from the binomial and
// multinomial laws of a uniformly random string's character counts.
func TestEntropyTableIsExact(t *testing.T) {
	for i, k := range entropyAlphabets {
		for n := entropyMinRun; n <= entropyMaxRow; n++ {
			mu, sigma := exactEntropyMoments(k, n)
			got := entropyTable[n-entropyMinRun][i]
			if math.Abs(got.mu-mu) > 1e-9*mu || math.Abs(got.sigma-sigma) > 1e-9*sigma {
				t.Errorf("k=%d n=%d: table (%v, %v), exact (%v, %v)", k, n, got.mu, got.sigma, mu, sigma)
			}
		}
	}
}

// exactEntropyMoments returns the mean and standard deviation, in bits per
// character, of the Shannon entropy of a uniformly random string of n
// characters over k symbols. With X_c the count of symbol c and g(x) =
// -(x/n) log2(x/n): E[H] = k E[g(X_1)], E[H^2] = k E[g(X_1)^2] +
// k(k-1) E[g(X_1) g(X_2)], X_1 ~ Bin(n, 1/k), X_2 | X_1=a ~ Bin(n-a, 1/(k-1)).
func exactEntropyMoments(k, n int) (float64, float64) {
	lf := make([]float64, n+1)
	for i := 1; i <= n; i++ {
		lf[i] = lf[i-1] + math.Log(float64(i))
	}
	g := func(x int) float64 {
		if x == 0 {
			return 0
		}
		p := float64(x) / float64(n)
		return -p * math.Log2(p)
	}
	binom := func(m, j int, p float64) float64 {
		return math.Exp(lf[m] - lf[j] - lf[m-j] + float64(j)*math.Log(p) + float64(m-j)*math.Log1p(-p))
	}
	var e1, e2, e12 float64
	for a := 0; a <= n; a++ {
		pa, ga := binom(n, a, 1/float64(k)), g(a)
		e1 += pa * ga
		e2 += pa * ga * ga
		var inner float64
		for b := 0; b <= n-a; b++ {
			inner += binom(n-a, b, 1/float64(k-1)) * g(b)
		}
		e12 += pa * ga * inner
	}
	kf := float64(k)
	mu := kf * e1
	return mu, math.Sqrt(kf*e2 + kf*(kf-1)*e12 - mu*mu)
}

// ordinaryPaths are real-world file names the deny list and the entropy
// test must keep (P07N-12): a Spring class and a Go test named for bearer
// tokens, file names that contain SG., dapi and hf_ inside or at the start
// of a word, and long snake and kebab names whose words the separated-run
// check reads one by one.
var ordinaryPaths = []string{
	"src/main/java/BearerTokenAuthenticationFilterTests.java",
	"internal/auth/bearer_token_authentication_test.go",
	"pkg/MSG.handler_registration_test.go",
	"clients/rapidapi_client_generated_models.py",
	"ml/hf_transformers_pipeline_utils.py",
	"internal/report/silent_failure_line_test.go",
	"config/configuration-management-service.yaml",
	"src/internationalization_helpers_test.ts",
	"docs/architecture_decision_records_index.md",
	"web/components/navigation-bar-dropdown-menu.tsx",
	"3f2504e0-4f89-11d3-9a0c-0305e82c3301/config.json",
	// Long words joined by a separator, which the joined-run check reads
	// as one run (P07N-7).
	"internal/authentication_configuration_loader.go",
	"src/integration-environment-provisioning.ts",
	"i18n/internationalization.localization_settings.json",
	"config/springframework.autoconfigure.properties",
	"src/FactoryBeanHandler.SingletonProxy.java",
	"migrations/20261013105232_add_subscription_tables.sql",
	"snapshots/TestRenderTemplates_NudgeWithPaths.golden",
	"releases/v1.27.2-rc.1+build.20261013/notes.md",
	// The spec's own: a file name holding "api" after a vendor prefix's
	// letters is a file name.
	"rapidapi_client.py",
}

// TestOrdinaryFileNamesAreKept: none of ordinaryPaths is changed.
func TestOrdinaryFileNamesAreKept(t *testing.T) {
	for _, p := range ordinaryPaths {
		if out, n := Redact(p); out != p || n != 0 {
			t.Errorf("Redact(%q) = (%q, %d), want it kept", p, out, n)
		}
	}
}

// TestPrefixedAndSeparatedTokensRedacted: a random token behind a vendor
// prefix or split by '-' or '_' into runs of 20 or more is redacted at
// least 99% of the time, bare and as a path segment (P07N-7): the gitleaks
// rules for Mailgun, Twilio and Postman keys, and the entropy test over
// each separated run and over a run without its letter prefix.
func TestPrefixedAndSeparatedTokensRedacted(t *testing.T) {
	const hex, upper36 = "0123456789abcdef", "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	r := rand.New(rand.NewPCG(31, 37))
	for _, c := range []struct {
		name string
		gen  func() string
	}{
		{"mailgun key-hex32", func() string { return "key-" + randomString(r, hex, 32) }},
		{"twilio SK+hex32", func() string { return "SK" + randomString(r, hex, 32) }},
		{"postman PMAK", func() string { return "PMAK-" + randomString(r, hex, 24) + "-" + randomString(r, hex, 34) }},
		{"hex64 split by underscore", func() string { return randomString(r, hex, 32) + "_" + randomString(r, hex, 32) }},
		{"newrelic NRAK-b36", func() string { return "NRAK-" + randomString(r, upper36, 27) }},
		{"tok_ hex24", func() string { return "tok_" + randomString(r, hex, 24) }},
		{"Tok hex24", func() string { return "Tok" + randomString(r, hex, 24) }},
	} {
		bare, path := 0, 0
		const n = 2000
		for i := 0; i < n; i++ {
			v := c.gen()
			if Secretish(v) {
				bare++
			}
			if Secretish("cfg/" + v) {
				path++
			}
		}
		if bs, ps := float64(bare)/n, float64(path)/n; bs < 0.99 || ps < 0.99 {
			t.Errorf("%s: %.4f redacted bare, %.4f as a path; want at least 0.99", c.name, bs, ps)
		}
	}
}

// TestProseIsScreenedWordByWord: the screen runs over text word by word, as
// the branch's Approved slot did (P07N-10), so a token, a key header or a
// password assignment inside a sentence is redacted and the sentence around
// it is kept.
func TestProseIsScreenedWordByWord(t *testing.T) {
	for _, s := range []string{
		"pass an argument list; for AWS calls use boto3", "use the staging profile",
		"region: us-east-1 (the default)", "the internationalization helpers",
	} {
		if out, n := Redact(s); out != s || n != 0 {
			t.Errorf("Redact(%q) = (%q, %d), want it kept", s, out, n)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 is the one", "token " + Redacted + " is the one"},
		{"-----BEGIN RSA PRIVATE" + " KEY-----", Redacted},
		{"DB_PASSWORD=hunter2", Redacted},
		{"use q8Zr4LmP0xT7vK2nW9yB3cHdQ1sE as the key", "use " + Redacted + " as the key"},
		{"AKIAIOSFODNN7EXAMPLE", Redacted},
	} {
		if out, n := Redact(tc.in); out != tc.want || n != 1 {
			t.Errorf("Redact(%q) = (%q, %d), want (%q, 1)", tc.in, out, n, tc.want)
		}
	}
}

// TestJoinedShortRunsRedacted: a random token cut by a separator into runs
// each shorter than 20 characters, so that neither 1.2.4's runs nor the
// separated-run check test any of them, is redacted at least 99% of the
// time bare and as a path segment: consecutive parts of 12 characters or
// more are tested joined (P07N-7). A UUID, whose groups are 8, 4, 4, 4 and
// 12 characters, is not joined and stays kept (ordinaryPaths).
func TestJoinedShortRunsRedacted(t *testing.T) {
	const hex = "0123456789abcdef"
	const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	r := rand.New(rand.NewPCG(41, 43))
	for _, c := range []struct {
		name string
		gen  func() string
	}{
		{"hex16-hex16", func() string { return randomString(r, hex, 16) + "-" + randomString(r, hex, 16) }},
		{"hex16.hex16", func() string { return randomString(r, hex, 16) + "." + randomString(r, hex, 16) }},
		{"hex16:hex16", func() string { return randomString(r, hex, 16) + ":" + randomString(r, hex, 16) }},
		{"hex12_hex12_hex12", func() string {
			return randomString(r, hex, 12) + "_" + randomString(r, hex, 12) + "_" + randomString(r, hex, 12)
		}},
		{"base62 19.19", func() string { return randomString(r, base62, 19) + "." + randomString(r, base62, 19) }},
		{"base62 19:19", func() string { return randomString(r, base62, 19) + ":" + randomString(r, base62, 19) }},
	} {
		bare, path := 0, 0
		const n = 2000
		for i := 0; i < n; i++ {
			v := c.gen()
			if Secretish(v) {
				bare++
			}
			if Secretish("cfg/" + v) {
				path++
			}
		}
		if bs, ps := float64(bare)/n, float64(path)/n; bs < 0.99 || ps < 0.99 {
			t.Errorf("%s: %.4f redacted bare, %.4f as a path; want at least 0.99", c.name, bs, ps)
		}
	}
}

// TestEachVendorRuleMatchesOnItsOwn: the deny list alone, with no entropy
// test behind it, refuses each gitleaks rule's own example, so a rule
// dropped from denyRE is visible even where the entropy test would still
// catch a random key (P07N-7: Mailgun, Twilio, Postman and the private-key
// header).
func TestEachVendorRuleMatchesOnItsOwn(t *testing.T) {
	for _, s := range []string{
		"key-" + "0000000000000000000000000000000a",
		"SK" + "00000000000000000000000000000000",
		"PMAK-" + "000000000000000000000000-0000000000000000000000000000000000",
		"-----BEGIN OPENSSH PRIVATE" + " KEY-----",
		"-----BEGIN PRIVATE" + " KEY-----",
	} {
		if !denyListed(s) {
			t.Errorf("denyListed(%q) = false", s)
		}
		if highEntropy(s) {
			t.Errorf("%q is high-entropy, so it does not test the deny rule alone", s)
		}
		if !Secretish(s) {
			t.Errorf("Secretish(%q) = false", s)
		}
	}
}

// identifierPaths are ordinary test-file names that 1.2.4's runs keep and
// that the slot layer's two additions refused when they were added: a part
// split at '_' tested alone (tokenInRun) and two camelCase parts tested
// joined (joinedRunHighEntropy), each part reading as words.
var identifierPaths = []string{
	"tests/test_OAuth2Client_refreshTokenExpiry.py",
	"src/test/java/com/acme/UserServiceImpl2Test_IntegrationSuite.java",
	"tests/unit/test_JsonWebToken_ExpiredSignature.py",
}

// TestIdentifierPartsAreKept: a part or joined run that reads as camelCase
// words is not secret-shaped under the slot layer's additions (P07N-7's
// measured cost), and the exemption is narrow: under 0.5% of random base62
// runs of 20 characters pass for a name, and a run with one letter case or
// with under three fifths lower-case letters never does.
func TestIdentifierPartsAreKept(t *testing.T) {
	for _, p := range identifierPaths {
		if Secretish(p) {
			t.Errorf("Secretish(%q) = true, want false", p)
		}
	}
	for _, s := range []string{"OAuth2Client", "UserServiceImpl2Test", "JsonWebTokenExpiredSignature", "HTTP2ServerPushTests"} {
		if !identifierShaped(s) {
			t.Errorf("%q does not read as words", s)
		}
	}
	const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	r := rand.New(rand.NewPCG(47, 53))
	words, n := 0, 20000
	for i := 0; i < n; i++ {
		if identifierShaped(randomString(r, base62, entropyMinRun)) {
			words++
		}
	}
	if rate := float64(words) / float64(n); rate >= 0.005 {
		t.Errorf("%.4f of random base62 runs of %d read as words; want under 0.005", rate, entropyMinRun)
	}
	for _, s := range []string{"0123456789abcdef0123", "ABCDEFGHIJKLMNOPQRST0123", "refreshtokenexpiry", "OAUTH2CLIENTSUITE", "AbCdEfGhIjKlMnOpQrSt", "HTTPSERVERPushTests"} {
		if identifierShaped(s) {
			t.Errorf("%q reads as words; a run with one letter case, or under three fifths lower-case, never does", s)
		}
	}
}

// legitimateIdentifiers are real values of the branch's identifier slots:
// Claude Code's task and background ids, built-in and plugin agent types,
// spec rule ids, program names, gates and mechanisms. The screen keeps all
// of them.
var legitimateIdentifiers = []string{
	"1", "42", "todo", "bash_1", "b8x2k4q9z", "TASK-123", "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
	"general-purpose", "Explore", "Plan", "statusline-setup", "output-style-setup",
	"superpowers:code-reviewer", "feature-dev:code-architect", "pr-review-toolkit:silent-failure-hunter",
	"res-13", "no-shell-true", "frozen-migrations", "no-truncate-shared-keyspaces",
	"forbid-direct-production-database-access-v2", "export-after-verify",
	"go", "golangci-lint", "python3", "x86_64-w64-mingw32-gcc", "clang++", "g++-13", "pre-commit",
	"docker-compose", "cargo-nextest", "llvm-objdump-18", "kubectl", "terraform", "aws", "uv",
	"go test", "npm test", "make test", "golangci-lint run", "cargo nextest", "pytest",
	"--password-stdin", "AWS_PROFILE", "aws-vault", "sso", "kubectl-oidc-login",
	"arn:aws:iam::123456789012:role/deploy", "vault:kv/data/ci/deploy", "env@AWS_PROFILE",
}

// TestIdentifiersKeptAndSecretsRedacted: every legitimate identifier is
// kept; a ghp_ token, an AWS key id, a Mailgun key, a 40-hex run, low-entropy
// ghp_ and Mailgun shapes only the deny list knows, and random 40-character
// values over the slots' own charsets are redacted (at least 99% of the
// random ones).
func TestIdentifiersKeptAndSecretsRedacted(t *testing.T) {
	for _, v := range legitimateIdentifiers {
		if out, n := Redact(v); out != v || n != 0 {
			t.Errorf("Redact(%q) = (%q, %d), want it kept", v, out, n)
		}
	}
	for _, s := range []string{
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"AKIAIOSFODNN7EXAMPLE",
		"key-" + "0123456789abcdef0123456789abcdef",
		"q8Zr4LmP0xT7vK2nW9yB3cHdJ5sF1gRe",
		"q8zr4lmp0xt7vk2nw9yb3chdj5sf1gre",
		"da39a3ee5e6b4b0d3255bfef95601890afd80709",
		// Low-entropy bodies the entropy test keeps, so only the deny list
		// refuses them.
		"ghp_" + strings.Repeat("a", 36),
		"key-" + strings.Repeat("0", 32),
	} {
		if out, n := Redact(s); out != Redacted || n != 1 {
			t.Errorf("Redact(%q) = (%q, %d), want it redacted whole", s, out, n)
		}
	}
	r := rand.New(rand.NewPCG(17, 29))
	for _, alphabet := range []string{
		"0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
		"0123456789abcdefghijklmnopqrstuvwxyz",
	} {
		const n = 500
		refused := 0
		for i := 0; i < n; i++ {
			if Secretish(randomString(r, alphabet, 40)) {
				refused++
			}
		}
		if share := float64(refused) / n; share < 0.99 {
			t.Errorf("%d symbols: %.3f of random 40-character values redacted, want at least 0.99", len(alphabet), share)
		}
	}
}

// TestDigestKeptOnlyUnderItsAlgorithm: a whole lowercase 40- or 64-hex path
// segment is kept only directly under "sha1" or "sha256", the directory
// that names its algorithm, at that algorithm's length. Anywhere else the
// same shape is redacted: a legacy GitHub token and a Datadog application
// key are 40 lowercase hex, a Linode token and a hex-encoded 32-byte key
// 64, and only the location tells a digest from them. The spec's case: a
// 64-hex sha kept inside sha256/<hex> and redacted bare.
func TestDigestKeptOnlyUnderItsAlgorithm(t *testing.T) {
	hex40 := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	hex64 := strings.Repeat("0123456789abcdef", 4)
	for _, p := range []string{
		"sha1/" + hex40, "store/sha1/" + hex40,
		"sha256/" + hex64, "blobs/sha256/" + hex64, "oci/blobs/sha256/" + hex64 + "/data",
	} {
		if out, n := Redact(p); out != p || n != 0 {
			t.Errorf("Redact(%q) = (%q, %d), want it kept: a digest under its algorithm's directory", p, out, n)
		}
	}
	for _, p := range []string{
		hex40, hex64, "objects/" + hex40, "config/" + hex64, ".tokens/" + hex40,
		"sha256/" + hex40, "sha1/" + hex64, "SHA256/" + hex64, "sha256x/" + hex64,
		"sha256/" + strings.ToUpper(hex64),
	} {
		if out, n := Redact(p); out != Redacted || n != 1 {
			t.Errorf("Redact(%q) = (%q, %d), want it redacted", p, out, n)
		}
	}
	// Inside a sentence: the digest's token is kept and the bare one is not.
	in := "pulled blobs/sha256/" + hex64 + " then saw " + hex64 + " in the log"
	want := "pulled blobs/sha256/" + hex64 + " then saw " + Redacted + " in the log"
	if out, n := Redact(in); out != want || n != 1 {
		t.Errorf("Redact(sentence) = (%q, %d), want (%q, 1)", out, n, want)
	}
}

// TestRedactSpansAndCounts: the spec's own cases. An exported token is
// redacted with the whole `NAME=value` word it sits in, so the value never
// survives beside a blanked name; an Authorization header's bearer token
// goes with its scheme word; a normal Python diff line is kept; n counts
// one replacement per token, deny-list matches and entropy hits together,
// and two deny matches inside one token count once.
func TestRedactSpansAndCounts(t *testing.T) {
	ghp := "ghp_" + strings.Repeat("Ab1", 12)
	for _, tc := range []struct {
		name, in, want string
		n              int
	}{
		{"exported token", "export GITHUB_TOKEN=" + ghp, "export " + Redacted, 1},
		{"exported token with a tail", "export GITHUB_TOKEN=" + ghp + " && go test", "export " + Redacted + " && go test", 1},
		{"bearer header", "Authorization: Bearer q8Zr4LmP0xT7vK2nW9yB3cHdJ5sF1gRe", "Authorization: " + Redacted, 1},
		{"bearer header, lowercase", "authorization: bearer " + strings.Repeat("a", 24), "authorization: " + Redacted, 1},
		{"password assignment keeps nothing of the value", "DB_PASSWORD=hunter2 ./run", Redacted + " ./run", 1},
		{"python diff lines", "-    print(f\"batch {i}\")\n+    log.info(\"batch %d\", i)", "-    print(f\"batch {i}\")\n+    log.info(\"batch %d\", i)", 0},
		{"a prompt", "Don't use print here; use the project logger", "Don't use print here; use the project logger", 0},
		{"two tokens", "AKIAIOSFODNN7EXAMPLE and " + ghp, Redacted + " and " + Redacted, 2},
		{"deny match and entropy hit", "key=" + ghp + "\nq8Zr4LmP0xT7vK2nW9yB3cHdJ5sF1gRe", Redacted + "\n" + Redacted, 2},
		{"aws secret in an assignment", "AWS_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Redacted, 1},
		{"leading and trailing blanks survive", "  " + ghp + "\t", "  " + Redacted + "\t", 1},
		{"empty", "", "", 0},
		{"user info in a url", "git clone https://alice:hunter2@github.com/acme/app.git", "git clone " + Redacted, 1},
	} {
		out, n := Redact(tc.in)
		if out != tc.want || n != tc.n {
			t.Errorf("%s: Redact(%q) = (%q, %d), want (%q, %d)", tc.name, tc.in, out, n, tc.want, tc.n)
		}
		if Secretish(tc.in) != (tc.n > 0) {
			t.Errorf("%s: Secretish = %v, want %v", tc.name, Secretish(tc.in), tc.n > 0)
		}
	}
	// A token Redacted itself is never redacted again: a second pass over
	// the output changes nothing, so a record screened twice reads the same.
	for _, s := range []string{"export GITHUB_TOKEN=" + ghp, "key=" + ghp + "\nq8Zr4LmP0xT7vK2nW9yB3cHdJ5sF1gRe"} {
		once, _ := Redact(s)
		if twice, n := Redact(once); twice != once || n != 0 {
			t.Errorf("Redact is not idempotent: %q -> %q (%d)", once, twice, n)
		}
	}
}

// TestDenyRulesSplitAsWritten: denyRE splits into one rule per denyTriggers
// entry, in the alternation's order, and each rule's own example (the
// branch's secretShaped list, one per rule) is matched by that rule, holds
// one of that rule's triggers, and comes out of Redact replaced. A rule
// whose trigger list missed a literal would still pass denyListed and fail
// here, which is the point: the triggers are what lets the hook skip the
// automaton, and they must never skip a match.
func TestDenyRulesSplitAsWritten(t *testing.T) {
	rules := denyRules()
	if len(rules) != 20 || len(rules) != len(denyTriggers) {
		t.Fatalf("denyRE split into %d rules; denyTriggers has %d", len(rules), len(denyTriggers))
	}
	examples := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"github_pat_11ABCDEFG0abcdefghijklmnop",
		"glpat-abcdefghijklmnopqrst",
		"xoxb-1234567890-abcdefghij",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
		"sk-proj-abcdefghijklmnopqrstuvwxyz",
		"sk_live_abcdefghijklmnop1234",
		"AIzaSyA1234567890abcdefghijklmnopqrstuv",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"npm_abcdefghijklmnopqrstuvwxyz0123456789",
		"pypi-AgEIcHlwaS5vcmc" + strings.Repeat("Ab1-", 13),
		"hvs.abcdefghijklmnopqrstuvwxyz",
		"dop_v1_" + strings.Repeat("ab12", 4),
		"hf_" + "abcdefghijklmnopqrstuvwxyzABCDEFGH",
		"key-" + "0123456789abcdef0123456789abcdef",
		"-----BEGIN PRIVATE" + " KEY-----",
		"user:token@host",
		"Bearer:abcdefghijklmnopqrstuvwxyz012345",
		"PGPASSWORD=hunter2",
	}
	for i, ex := range examples {
		r := rules[i]
		if !r.re.get().MatchString(ex) {
			t.Errorf("rule %d (%s) does not match its own example %q", i, r.re.pattern, ex)
		}
		if !r.triggered(ex, strings.ToLower(ex)) {
			t.Errorf("rule %d: example %q holds none of its triggers %v %v", i, ex, r.any, r.folded)
		}
		if out, n := Redact("x " + ex + " y"); out != "x "+Redacted+" y" || n != 1 {
			t.Errorf("rule %d: Redact(%q) = (%q, %d), want the example redacted", i, ex, out, n)
		}
	}
	// A line with no trigger runs no rule: the deny spans are empty however
	// the line is shaped, and the whole alternation agrees.
	for _, s := range []string{"the logger goes through emit", "x = f(a, b) + 1", "git commit -m 'fix the build'"} {
		if sp := denySpans(s); len(sp) != 0 {
			t.Errorf("denySpans(%q) = %v, want none", s, sp)
		}
		if denyRE.get().MatchString(s) {
			t.Errorf("denyRE matches %q, which has no trigger: a trigger is missing", s)
		}
	}
}

// TestKeyRuleByHandMatchesTheRegexp: keySpans ends a match exactly where
// denyRE's last rule does, on the branch's corpus, on the shapes the rule
// was written for, and on random lines built from key words, separators,
// blanks and noise in both letter cases. Break: drop a word from keyWords,
// or let a blank between the key and its `=` end the search, and a line the
// regexp refuses is kept.
func TestKeyRuleByHandMatchesTheRegexp(t *testing.T) {
	rules := denyRules()
	re := rules[len(rules)-1].re.get()
	ends := func(spans [][]int) []int {
		var out []int
		for _, sp := range spans {
			out = append(out, sp[1])
		}
		return out
	}
	same := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	check := func(s string) {
		t.Helper()
		got, want := ends(keySpans(asciiLower(s))), ends(re.FindAllStringIndex(s, -1))
		if !same(got, want) {
			t.Errorf("keySpans(%q) ends at %v, the regexp at %v", s, got, want)
		}
	}
	corpus := append([]string{}, secretShaped...)
	corpus = append(corpus, ordinaryPaths...)
	corpus = append(corpus, legitimateIdentifiers...)
	corpus = append(corpus,
		"PASSWORD = x", "password\t=x", "my-password:1", "xdatabase_url=1", "DATABASE_URL: postgres://x",
		"token=token=", "password==", "secret = = x", "_pass_pwd=", "passwordpassword=",
		"a_key=b_token=c", "api_key:abc", "cookie:", "the token: is here", "tokens=1", "key=1",
		"Authorization: Bearer x", "credentials=", "credential=", "accountkey=1", "_dsn =", "foo_pat:",
		"x=1; y=2", "if a == b:", "label: value", "pass:", "passwd:", "passphrase =",
	)
	for _, s := range corpus {
		check(s)
	}
	r := rand.New(rand.NewPCG(3, 5))
	pieces := []string{"password", "TOKEN", "_key", "secret", "_pass", "Api_Key", "cookie", "accountkey", "credential", "database_url",
		"=", ":", " ", "  ", "\t", "_", "-", "x", "ab", "Zq", "9", "/", ".", "token", "pass", "key", "auth", "_auth", "pwd", "_pwd"}
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		for k, n := 0, 1+r.IntN(8); k < n; k++ {
			b.WriteString(pieces[r.IntN(len(pieces))])
		}
		check(b.String())
	}
}

// raceDetector reports a test binary built with -race, whose instrumentation
// slows this code ten to fifteen times (measured: 2.0 ms to 24 ms for the
// 8,000-byte input below, before the key rule was written by hand). A
// timing budget is a budget for the binary a hook runs, so under the
// detector it is scaled rather than dropped.
func raceDetector() bool {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range bi.Settings {
		if s.Key == "-race" {
			return s.Value == "true"
		}
	}
	return false
}

// raceFactor is the budget multiplier under the race detector.
const raceFactor = 10

// TestRedactIsFastEnoughForTheHook: an 8,000-byte input, the prompt prefix
// the memory prototype screened on UserPromptSubmit, is redacted in under 5
// ms, the figure its spec set for that hook path's share of a 150 ms budget.
// Nothing shipped calls Redact from a hook yet; the budget is kept so that
// whatever does first inherits a measured cost, not a guess. The input is a prompt as prompts come: a hundred lines of prose
// and code, a third of them holding a trigger word and two in six a secret,
// so the trigger search runs on every line and a rule on a third. Measured
// 0.6 ms here; the whole alternation over the same bytes as one line costs
// 11 ms, which is why Redact runs rule by rule and line by line. The
// regexps compile on first use and that compile is paid before the clock
// starts, as it is in the hook after its first call.
//
// Timed as the best of five calls, as the prototype's delivery budget was, not
// their mean: on the four-core runner the mean of five was 11.4 ms against the
// 5 ms budget while four mutation sweeps shared the machine, and the best
// of five measured 0.7 to 1.3 ms with eight busy loops on its four cores.
// The mean also counts the scheduler; the best is the call's own cost, and
// the one-line alternation's 11 ms is over the budget on every call.
func TestRedactIsFastEnoughForTheHook(t *testing.T) {
	var b strings.Builder
	lines := []string{
		"Don't use print here; use the project logger and keep the output structured.",
		"    log.info(\"batch %d of %d written to %s\", i, n, path)  # not print(f\"...\")",
		"The diagnostic and progress output goes through emit in app/diag.py as JSON.",
		"export DB_PASSWORD=hunter2 && ./run --once",
		"for row in rows: handle(row, retries=3, timeout=30, verbose=False)",
		"aws configure set aws_access_key_id AKIAIOSFODNN7EXAMPLE --profile dev",
	}
	for i := 0; b.Len() < 8000; i++ {
		b.WriteString(lines[i%len(lines)])
		b.WriteByte('\n')
	}
	in := b.String()[:8000]
	if _, n := Redact(in); n == 0 {
		t.Fatal("the timed input was not screened at all")
	}
	budget := 5 * time.Millisecond
	if raceDetector() {
		budget *= raceFactor
	}
	const runs = 5
	best := time.Duration(1<<63 - 1)
	for i := 0; i < runs; i++ {
		start := time.Now()
		Redact(in)
		if d := time.Since(start); d < best {
			best = d
		}
	}
	t.Logf("Redact of 8,000 bytes: best of %d calls %v", runs, best)
	if best > budget {
		t.Errorf("Redact of 8,000 bytes took %v at best of %d, want under %v", best, runs, budget)
	}
}

// TestSecretNamedIsTheKeyRule: SecretNamed answers for a bare member name
// exactly what denyRE's last rule answers for that name followed by `=`, on
// the key corpus and the shapes a structured payload carries, so a reader
// that walks JSON members and a reader of `NAME=value` text refuse the same
// names. The names a hook payload or a transcript carries by the hundred
// (input_tokens, cache_read_input_tokens, apiKeySource, tool_use_id) are
// not secret names, and the ones a credential sits under are.
func TestSecretNamedIsTheKeyRule(t *testing.T) {
	rules := denyRules()
	re := rules[len(rules)-1].re.get()
	for _, k := range []string{
		"password", "DB_PASSWORD", "api_key", "apiKey", "ApiKey", "client_secret", "GITHUB_TOKEN",
		"aws_secret_access_key", "authorization_token", "session_key", "cookie", "passphrase",
		"credentials", "x_auth", "my_pat", "sentry_dsn", "private_key", "PGPASSWORD",
		"input_tokens", "cache_read_input_tokens", "output_tokens", "apiKeySource", "tool_use_id",
		"session_id", "transcript_path", "hook_event_name", "tokens", "token_count", "keys",
		"leaf_hash", "prev_chain", "cwd", "", "key",
	} {
		want := re.MatchString(k + "=")
		if got := SecretNamed(k); got != want {
			t.Errorf("SecretNamed(%q) = %v; the rule says %v for %q", k, got, want, k+"=")
		}
	}
	for k, want := range map[string]bool{"password": true, "GITHUB_TOKEN": true, "input_tokens": false, "apiKeySource": false} {
		if SecretNamed(k) != want {
			t.Errorf("SecretNamed(%q) = %v, want %v", k, !want, want)
		}
	}
}

// TestRedact_ASecretNamedKeyTakesItsValue: a secret-named key's value is
// redacted however the key and value are joined -- `:` or `=`, with blanks
// on either side, or as a JSON member whose name is quoted -- and Secretish
// says so. The key rule's match ended at the `:` or `=`, and widening took
// only the token holding it, so `password: hunter2` kept hunter2, and a
// quoted member never matched at all.
func TestRedact_ASecretNamedKeyTakesItsValue(t *testing.T) {
	for _, line := range []string{
		"password: hunter2",
		"password = hunter2",
		"password =hunter2",
		"DB_PASSWORD : hunter2",
		`{"password": "hunter2"}`,
		`{"password":"hunter2","user":"x"}`,
		`"client_secret" = "hunter2"`,
		`password: "and more hunter2"`,
		"api_key:\thunter2",
	} {
		out, n := Redact(line)
		if strings.Contains(out, "hunter2") || n == 0 {
			t.Errorf("Redact(%q) = %q, %d: the value is kept", line, out, n)
		}
		if !Secretish(line) {
			t.Errorf("Secretish(%q) = false", line)
		}
	}
	for _, line := range []string{"the password policy: 12 characters", `{"user": "x"}`} {
		if out, n := Redact(line); n != 0 {
			t.Errorf("Redact(%q) = %q, %d; want it unchanged", line, out, n)
		}
	}
}
