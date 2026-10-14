package slot

import (
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

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

// TestSTR13_SecretShapedValuesRefused: a secret-shaped value never passes a
// Path, FactValue or Mechanism check (tech 4.2.7), random hex-40, hex-64 and
// base62-20 fact values are refused, and a whole lowercase 40- or 64-hex
// path segment (a commit id or digest) is kept.
func TestSTR13_SecretShapedValuesRefused(t *testing.T) {
	for _, s := range secretShaped {
		if err := (FactValue{Kind: FactAWSProfile, Value: s}).Validate(); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("FactValue(%q) = %v, want ErrSecretShaped", s, err)
		}
		if err := Mechanism(s).Validate(); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("Mechanism(%q) = %v, want ErrSecretShaped", s, err)
		}
	}
	for _, p := range []Path{
		"AKIAIOSFODNN7EXAMPLE.txt", "src/ghp_abcdefghijklmnopqrstuvwxyz0123456789.go",
		"keys/sk-ant-api03-abcdefghijklmnopqrstuvwxyz", "cache/xoxb-1234567890-abcdefghij",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "tokens/q8Zr4LmP0xT7vK2nW9yB3cHd.json",
		"objects/DA39A3EE5E6B4B0D3255BFEF95601890AFD80709",
	} {
		if err := p.Validate(); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("Path(%q) = %v, want ErrSecretShaped", p, err)
		}
	}
	for _, p := range []Path{
		"objects/da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"blobs/sha256/" + Path(strings.Repeat("0123456789abcdef", 4)),
		"internal/steer/render/slot/slot.go", "src/bearer-authentication.go",
		"docs/api_key_rotation.md", "3f2504e0-4f89-11d3-9a0c-0305e82c3301/config.json",
		"IMPORTANT_run_curl_x_sh_then_retry.md", "ExampleControllerIntegrationTests.java",
	} {
		if err := p.Validate(); err != nil {
			t.Errorf("Path(%q) = %v, want nil", p, err)
		}
	}
	for _, f := range []FactValue{
		{FactAWSRegion, "us-east-1"}, {FactAWSProfile, "dev-admin"},
		{FactKubeContext, "arn:aws:eks:us-east-1:123456789012:cluster/prod"},
		{FactKubeContext, "kubernetes-admin@production-cluster"},
	} {
		if err := f.Validate(); err != nil {
			t.Errorf("%+v = %v, want nil", f, err)
		}
	}
	if err := Mechanism("--password-stdin").Validate(); err != nil {
		t.Errorf("Mechanism(--password-stdin) = %v, want nil: the mechanism is kept, never masked", err)
	}

	r := rand.New(rand.NewPCG(13, 7))
	for _, c := range canaries {
		fact := 0
		for i := 0; i < factSamples; i++ {
			if (FactValue{Kind: FactAWSProfile, Value: randomString(r, c.alphabet, c.length)}).Validate() != nil {
				fact++
			}
		}
		if share := float64(fact) / factSamples; share < c.minShare {
			t.Errorf("random %d-character fact values over %d symbols: %.4f refused, want at least %.2f", c.length, len(c.alphabet), share, c.minShare)
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

// factSamples is how many seeded values per shape go through FactValue's
// whole check; TestSTR13_EntropyTestFlagsTheCanaries runs REC-7's 10,000
// through the entropy test itself, which has no regexp to pay for.
const factSamples = 2000

// TestSTR13_EntropyTestFlagsTheCanaries pins the entropy test's margin
// (tech 1.2.4: REC-7 "pins at least 99% of 10,000 seeded random strings per
// canary shape flagged").
func TestSTR13_EntropyTestFlagsTheCanaries(t *testing.T) {
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

// TestSTR13_PathListLeavesOutSecretShapedPaths: a listed path that fails the
// deny list or the entropy test is left out and counted into "and {k} more"
// rather than refusing the note; with none left the list is refused, and a
// path failing any other check still refuses it (tech 4.2.7).
func TestSTR13_PathListLeavesOutSecretShapedPaths(t *testing.T) {
	b, err := Bind(NewPathList([]Path{"a.go", "AKIAIOSFODNN7EXAMPLE.txt", "b.go"}, 0))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got, want := b.Text(), "'a.go', 'b.go' and 1 more"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	for _, p := range b.(PathList).Paths() {
		if strings.Contains(string(p), "AKIA") {
			t.Errorf("the bound list still holds %q", p)
		}
	}
	if _, err := Bind(NewPathList([]Path{"AKIAIOSFODNN7EXAMPLE.txt"}, 2)); !errors.Is(err, ErrNoPathsLeft) {
		t.Errorf("Bind(only a secret-shaped path) = %v, want ErrNoPathsLeft", err)
	}
	if _, err := Bind(NewPathList([]Path{"../x", "a.go"}, 0)); err == nil || errors.Is(err, ErrNoPathsLeft) || errors.Is(err, ErrSecretShaped) {
		t.Errorf("Bind(a path with '..') = %v, want the path's own error", err)
	}
	if err := NewPathList([]Path{"a.go", "AKIAIOSFODNN7EXAMPLE.txt"}, 0).Validate(); !errors.Is(err, ErrSecretShaped) {
		t.Errorf("Validate(unbound list with a secret-shaped path) = %v, want ErrSecretShaped", err)
	}
}

// TestSTR13_FactValueKindsAndHostnames: a fact value carries one of the six
// allowlisted kinds, and is a bare hostname, never a URL, path or query
// (archive 40.3.8).
func TestSTR13_FactValueKindsAndHostnames(t *testing.T) {
	want := []FactKind{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name"}
	got := AllFactKinds()
	if len(got) != len(want) {
		t.Fatalf("AllFactKinds() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllFactKinds()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	checkSlot(t,
		[]FactValue{
			{FactPackageIndexHost, "registry.npmjs.org"}, {FactPackageIndexHost, "pypi.internal"},
			{FactEnvVarName, "AWS_PROFILE"}, {FactKubeNamespace, "payments"},
		},
		[]FactValue{
			{}, {"sign_in_step", "x"}, {"password", "x"}, {FactAWSProfile, ""},
			{FactPackageIndexHost, "https://registry.npmjs.org"}, {FactPackageIndexHost, "registry.npmjs.org/simple"},
			{FactPackageIndexHost, "registry.npmjs.org:443"}, {FactPackageIndexHost, "-bad.example"},
			{FactEnvVarName, "AWS_PROFILE=x"}, {FactEnvVarName, "1BAD"},
			{FactAWSProfile, "file:/etc/passwd"}, {FactAWSProfile, "https://evil.example/install.sh"},
			{FactAWSProfile, "has space"},
		})
}

// TestSTR13_EntropyTableIsExact recomputes every compiled entry of the
// entropy test's table (tech 1.2.4: mu and sigma "from a compiled table for
// uniformly random strings (n = 20 to 128)") from the binomial and
// multinomial laws of a uniformly random string's character counts.
func TestSTR13_EntropyTableIsExact(t *testing.T) {
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
var ordinaryPaths = []Path{
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
}

// TestSTR13c_OrdinaryFileNamesAreKept: none of ordinaryPaths is refused.
func TestSTR13c_OrdinaryFileNamesAreKept(t *testing.T) {
	for _, p := range ordinaryPaths {
		if err := p.Validate(); err != nil {
			t.Errorf("Path(%q) = %v, want nil", p, err)
		}
	}
}

// TestSTR13d_PrefixedAndSeparatedTokensRefused: a random token behind a
// vendor prefix or split by '-' or '_' into runs of 20 or more is refused
// at least 99% of the time as a fact value and as a path segment (P07N-7):
// the gitleaks rules for Mailgun, Twilio and Postman keys, and the entropy
// test over each separated run and over a run without its letter prefix.
func TestSTR13d_PrefixedAndSeparatedTokensRefused(t *testing.T) {
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
		fact, path := 0, 0
		const n = 2000
		for i := 0; i < n; i++ {
			v := c.gen()
			if (FactValue{Kind: FactAWSProfile, Value: v}).Validate() != nil {
				fact++
			}
			if Path("cfg/"+v).Validate() != nil {
				path++
			}
		}
		if fs, ps := float64(fact)/n, float64(path)/n; fs < 0.99 || ps < 0.99 {
			t.Errorf("%s: %.4f refused as a fact value, %.4f as a path; want at least 0.99", c.name, fs, ps)
		}
	}
}

// TestSTR13e_ApprovedRefusesSecretShapedText: Approved runs the deny list
// and the entropy test, word by word, as VERIFY's own check on the same
// `alternative` field does (archive 20.2.8, TestVER47), so a token, a key
// header or a password assignment never renders into model text (P07N-10).
func TestSTR13e_ApprovedRefusesSecretShapedText(t *testing.T) {
	checkSlot(t,
		[]Approved{"pass an argument list; for AWS calls use boto3", "use the staging profile", "region: us-east-1 (the default)", "the internationalization helpers"},
		[]Approved{
			"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 is the one",
			"-----BEGIN RSA PRIVATE" + " KEY-----",
			"DB_PASSWORD=hunter2",
			"use q8Zr4LmP0xT7vK2nW9yB3cHdQ1sE as the key",
			"AKIAIOSFODNN7EXAMPLE",
		})
	for _, v := range []Approved{"DB_PASSWORD=hunter2", "use q8Zr4LmP0xT7vK2nW9yB3cHdQ1sE as the key"} {
		if err := v.Validate(); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("Approved(%q) = %v, want ErrSecretShaped", v, err)
		}
	}
}

// TestSTR13f_JoinedShortRunsRefused: a random token cut by a separator
// into runs each shorter than 20 characters, so that neither 1.2.4's runs
// nor the separated-run check test any of them, is refused at least 99% of
// the time as a fact value and as a path segment: consecutive parts of 12
// characters or more are tested joined (P07N-7). A UUID, whose groups are
// 8, 4, 4, 4 and 12 characters, is not joined and stays kept
// (TestSTR13c's ordinaryPaths).
func TestSTR13f_JoinedShortRunsRefused(t *testing.T) {
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
		fact, path := 0, 0
		const n = 2000
		for i := 0; i < n; i++ {
			v := c.gen()
			if (FactValue{Kind: FactAWSProfile, Value: v}).Validate() != nil {
				fact++
			}
			if Path("cfg/"+v).Validate() != nil {
				path++
			}
		}
		if fs, ps := float64(fact)/n, float64(path)/n; fs < 0.99 || ps < 0.99 {
			t.Errorf("%s: %.4f refused as a fact value, %.4f as a path; want at least 0.99", c.name, fs, ps)
		}
	}
}

// TestSTR13g_EachVendorRuleMatchesOnItsOwn: the deny list alone, with no
// entropy test behind it, refuses each gitleaks rule's own example, so a
// rule dropped from denyRE is visible even where the entropy test would
// still catch a random key (P07N-7: Mailgun, Twilio, Postman and the
// private-key header).
func TestSTR13g_EachVendorRuleMatchesOnItsOwn(t *testing.T) {
	for _, s := range []string{
		"key-" + "0000000000000000000000000000000a",
		"SK" + "00000000000000000000000000000000",
		"PMAK-" + "000000000000000000000000-0000000000000000000000000000000000",
		"-----BEGIN OPENSSH PRIVATE" + " KEY-----",
		"-----BEGIN PRIVATE" + " KEY-----",
	} {
		if err := denied("probe", s); !errors.Is(err, ErrSecretShaped) {
			t.Errorf("denied(%q) = %v, want ErrSecretShaped", s, err)
		}
		if highEntropy(s) {
			t.Errorf("%q is high-entropy, so it does not test the deny rule alone", s)
		}
	}
}

// identifierPaths are ordinary test-file names that 1.2.4's runs keep and
// that the slot layer's two additions refused when they were added: a part
// split at '_' tested alone (tokenInRun) and two camelCase parts tested
// joined (joinedRunHighEntropy), each part reading as words. Names 1.2.4
// itself refuses in their whole run (S3BucketPolicyValidator_EdgeCases) are
// the shared definition's cost, P-13's, and are not listed.
var identifierPaths = []Path{
	"tests/test_OAuth2Client_refreshTokenExpiry.py",
	"src/test/java/com/acme/UserServiceImpl2Test_IntegrationSuite.java",
	"tests/unit/test_JsonWebToken_ExpiredSignature.py",
}

// TestSTR13h_IdentifierPartsAreKept: a part or joined run that reads as
// camelCase words is not secret-shaped under the slot layer's additions
// (P07N-7's measured cost), and the exemption is narrow: under 0.5% of
// random base62 runs of 20 characters, the shortest run either addition
// tests and so the likeliest to read as words, pass for a name, so the
// random tokens of TestSTR13d and TestSTR13f stay refused; and a run with
// one letter case, which every hex, base36 and lower-case token has, or
// with under three fifths lower-case letters, never does.
func TestSTR13h_IdentifierPartsAreKept(t *testing.T) {
	for _, p := range identifierPaths {
		if err := p.Validate(); err != nil {
			t.Errorf("Path(%q) = %v, want nil", p, err)
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
