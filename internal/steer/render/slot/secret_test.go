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
	"hf_abcdefghijklmnopqrstuvwx",
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
