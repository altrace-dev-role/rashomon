package shape

import (
	"encoding/json"
	"path"
	"strings"
)

// Labels. A closed set, for the same reason the verb classes are one: the
// vocabulary must never grow to fit the input, because a label that can carry a
// value from the path is a path that reached the record by another name.
//
// A label says what a path LOOKS LIKE. It is not a finding, it does not say the
// file was secret, and it does not say what the call did to it. Nothing here
// renders with an adjective.
const (
	LabelSSHKey           = "ssh-key"
	LabelCredentialShaped = "credential-shaped"
	LabelCloudConfig      = "cloud-config"
	LabelEnvFile          = "env-file"
	LabelCertificate      = "certificate"

	// LabelTestFile is a basename that a test framework's own naming
	// convention reserves for tests: foo_test.go, test_foo.py, foo.spec.ts,
	// FooTest.java. It says the path is named like a test, not that the file
	// holds one, and it is what lets a failing test run followed by edits to
	// test files only be told apart from one followed by a fix.
	LabelTestFile = "test-file"

	// LabelNone is a path that matched no row: the tool named a file and the
	// table recognised nothing about it.
	LabelNone = "none"
	// LabelUnknown is a path that could not be read at all -- absent,
	// non-string, or empty. It is distinct from LabelNone, which is a real
	// answer about a real path.
	LabelUnknown = "unknown"
)

// Labels is the vocabulary, for the schema enum test to compare against.
//
// It exists for the reason store.Reasons() and report.Reasons() exist: a
// vocabulary that is not exported through a function drifts from the published
// schema silently, and the drift is only visible once a record in the field
// carries a value the schema forbids.
func Labels() []string {
	return []string{
		LabelSSHKey,
		LabelCredentialShaped,
		LabelCloudConfig,
		LabelEnvFile,
		LabelCertificate,
		LabelTestFile,
		LabelNone,
		LabelUnknown,
	}
}

// pathFields are the tools whose input names a file, and the ONE field read for
// each. Nothing else in any input is read by this file.
//
// The list is the specification's, exactly: Read, Edit and Write carry
// file_path, NotebookEdit carries notebook_path. MultiEdit also carries a
// file_path and is deliberately absent -- version 1 labels the four tools the
// specification names, and widening the contract is a specification change, not
// an implementation detail.
//
// Bash is absent on purpose and not by omission: extracting a file target from
// a command line is the recognised-file-command problem, and a label from a
// guess would be a guess.
var pathFields = map[string]string{
	"Read":         "file_path",
	"Edit":         "file_path",
	"Write":        "file_path",
	"NotebookEdit": "notebook_path",
}

// labelRule is one row of the table. Matching is on the case-folded basename
// alone; exact, then prefix, then suffix. The cased fields are the one
// exception to the folding: an exact name, a suffix, and a prefix and a suffix
// together (casedBoth), matched against the basename as written, for
// conventions whose own tool matches them case-sensitively -- see the
// test-file row.
type labelRule struct {
	label       string
	exact       []string
	prefix      []string
	suffix      []string
	casedExact  []string
	casedSuffix []string
	casedBoth   [][2]string
}

// labelTable is the fixed pattern list, in evaluation order. The FIRST row that
// matches decides, so the order is part of the contract and a new row goes
// where its convention belongs rather than at the end.
//
// Every row cites the convention it matches. A row is added by naming a
// convention; it is never added by widening a pattern until a path matches.
var labelTable = []labelRule{
	// OpenSSH keeps its key pair under a fixed basename per algorithm
	// (ssh-keygen(1): id_rsa, id_dsa, id_ecdsa, id_ed25519, and the _sk
	// variants), with the public half suffixed .pub. Prefixes rather than
	// exact names, so an id_ed25519_work or an id_rsa.pub still reads as what
	// it is. The label does not separate the public half from the private one:
	// the basename convention does not reliably say which, and the label is
	// not a finding.
	{
		label:  LabelSSHKey,
		exact:  []string{"authorized_keys"},
		prefix: []string{"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519"},
	},

	// dotenv, and direnv's .envrc.
	//
	// Exact names plus a dotted or dashed prefix, not a bare ".env" prefix:
	// that would swallow .environment, .envelope and .envision, which is the
	// same prose-matching the credential row below refuses to do.
	{
		label:  LabelEnvFile,
		exact:  []string{".env", ".envrc"},
		prefix: []string{".env.", ".env-"},
	},

	// Provider credential and config files, by their documented basenames:
	// kubectl's kubeconfig, gcloud's application_default_credentials.json,
	// gsutil's .boto, s3cmd's .s3cfg, and Terraform state and variable files,
	// which hold provider secrets in cleartext.
	//
	// .tfstate.backup is listed beside .tfstate because terraform apply
	// writes it on every run and it holds the same cleartext secrets. A row
	// that names state files and misses the copy of the state file made every
	// time state changes is a row that misses its own stated target.
	//
	// This row precedes credential-shaped so that a provider's own credential
	// file reads as the provider's, not as the generic shape.
	{
		label: LabelCloudConfig,
		exact: []string{"kubeconfig", "application_default_credentials.json", ".boto", ".s3cfg"},
		suffix: []string{
			".tfstate", ".tfstate.backup", ".tfstate.bak", ".tfvars", ".kubeconfig",
		},
	},

	// Tokens, API keys, passwords and keychains, by the basenames the tools
	// that read them document: netrc(5) and its Windows spelling, libpq's
	// .pgpass, git-credential-store's .git-credentials, Apache's .htpasswd,
	// the AWS CLI's credentials file, npm's .npmrc, twine's .pypirc, Docker's
	// legacy .dockercfg, GnuPG's secring.gpg, KeePass's .kdbx, and Apple's
	// keychain files.
	//
	// "credentials." is a prefix as well as an exact name: GCP service-account
	// keys ship as credentials.json and Rails writes credentials.yml.enc, and
	// the bare name alone would miss both.
	//
	// The secret prefix is "secret." and "secrets.", never a bare "secret": a
	// bare prefix would swallow secret-santa.md and secret_handshake.go, and a
	// row that matches prose is a row that was widened until something
	// matched.
	//
	// .asc is deliberately NOT here. It is the armoured-output extension, and
	// what it most often names is a detached signature, which is public by
	// construction; labelling one credential-shaped would be a false claim
	// about the commonest file carrying that suffix.
	//
	// This row now precedes the certificate row, which it did not at first.
	// The certificate row matches on suffix alone, so with the old order
	// secret.key, credentials.pem and kubeconfig.pem all read as certificate:
	// a name that says exactly what the file is, losing to an extension that
	// says only how it is encoded.
	{
		label: LabelCredentialShaped,
		exact: []string{
			".netrc", "_netrc", ".pgpass", ".git-credentials", ".htpasswd",
			"credentials", ".npmrc", ".pypirc", ".dockercfg", "secring.gpg",
			"secret", "secrets",
		},
		prefix: []string{"secret.", "secrets.", "credentials."},
		suffix: []string{".kdbx", ".keychain", ".keychain-db", ".gpg", ".token"},
	},

	// X.509 material, certificates and their private keys together: PEM and
	// DER encodings, PKCS#12 and PKCS#7 bundles, Java keystores, and the .key
	// and .csr halves of the same convention. One row because they are one
	// naming family; the vocabulary has no separate private-key value and
	// inventing one here would widen a closed set.
	//
	// .key is known to collide with Apple Keynote, which this tool will meet
	// on a laptop. It stays, and the collision is recorded rather than hidden:
	// server.key and tls.key are the commoner meaning on a machine running an
	// agent, a label is not a finding, and dropping the suffix would trade a
	// harmless mislabel for a missed private key. Last of the sensitive rows,
	// so any basename that names a credential outright is claimed before the
	// extension is consulted.
	{
		label:  LabelCertificate,
		suffix: []string{".pem", ".crt", ".cer", ".der", ".p12", ".pfx", ".p7b", ".jks", ".keystore", ".key", ".csr"},
	},

	// Test files, by the basename each framework's runner looks for: go
	// test's _test.go; pytest's test_*.py, *_test.py and conftest.py; the
	// .test. and .spec. infixes Jest, Vitest and Mocha default to, with the
	// .mts and .cts extensions Vitest also reads; RSpec's _spec.rb;
	// GoogleTest's _test.cc and _unittest.cc, the convention ctest projects
	// follow; and the JUnit, Kotlin and .NET class-name suffixes.
	//
	// AFTER every sensitive row, and that order is the point: a secret kept
	// in a test directory, or named like a fixture, is still a secret, and a
	// .env.test.js is an env file first.
	//
	// The class-name suffixes are matched as written (cased), not folded.
	// Folded, *Test.java is a suffix "test.java" and swallows Latest.java and
	// Contest.java, and *Tests.cs swallows Contests.cs -- source files the
	// detections built on this label would then read as tests, which is the
	// over-claim this label exists to avoid. The JUnit convention is
	// case-sensitive by definition (a class name), so the case is part of it;
	// PHPUnit's *Test.php likewise. *Test still over-matches a class whose
	// name only ends in Test (ABTest.java, a source file for an A/B test):
	// that is kept, and recorded, rather than guessed around.
	//
	// go test's and pytest's names are cased too, and GoogleTest's with them,
	// because those runners
	// match them case-sensitively: go test reads calc_TEST.go as source and
	// pytest does not collect TEST_x.py. Folded, the label would call them
	// tests to a detection that exists to tell tests from source. The .test.
	// and .spec. infixes and RSpec's _spec.rb stay folded.
	//
	// Directory rules (tests/, __tests__/) are left out: this table matches
	// basenames by design, and a fixture in tests/ is not a test.
	{
		label: LabelTestFile,
		suffix: []string{
			"_spec.rb",
			".test.js", ".test.jsx", ".test.ts", ".test.tsx", ".test.mjs", ".test.cjs", ".test.mts", ".test.cts",
			".spec.js", ".spec.jsx", ".spec.ts", ".spec.tsx", ".spec.mjs", ".spec.cjs", ".spec.mts", ".spec.cts",
		},
		casedExact: []string{"conftest.py"},
		casedSuffix: []string{
			"_test.go", "_test.py", "_test.cc", "_unittest.cc",
			"Test.java", "Tests.java", "Test.kt", "Tests.kt", "Test.cs", "Tests.cs", "Test.php",
		},
		casedBoth: [][2]string{{"test_", ".py"}},
	},
}

// Label reports what the file a call names looks like.
//
// The empty string means this tool names no file, and is what a Bash, Agent or
// WebFetch call gets: the record's field is null, which is not the same
// statement as LabelUnknown. Every other return is one of Labels().
//
// The path itself never leaves this function. Only the basename is examined,
// only against the fixed table above, and nothing derived from it is returned:
// the result is a constant from the closed vocabulary or nothing at all.
//
// There is no filesystem access. The path is not opened, stat'd or resolved,
// and this package imports nothing that could.
func Label(toolName string, toolInput json.RawMessage) string {
	field, ok := pathFields[toolName]
	if !ok {
		return ""
	}
	// stringField, not a reader of its own. A second copy of it here would
	// put two functions that look inside tool_input in the one file that
	// exists so that only one has to be audited, which is the claim at the
	// top of shape.go and is worth more than the four lines a local copy
	// would save.
	//
	// A non-object input, a missing field, a field that is not a string, and
	// an empty one all arrive here as absent, and all are LabelUnknown: "this
	// tool should have named a file and this input does not" is a real
	// outcome, and it is not LabelNone.
	p, ok := stringField(toolInput, field)
	if !ok || p == "" {
		return LabelUnknown
	}
	return labelForBase(basename(p))
}

// basename reduces a path to the last element, as written. labelForBase folds
// its case, and keeps the written form only for the table's cased suffixes.
//
// A leading ~ is dropped rather than resolved. Expanding it would mean reading
// the user's home directory, and it would change nothing that is examined here:
// the basename of ~/.ssh/id_rsa is id_rsa either way. The anchor a ~ implies
// matters to a path RULE, which is Part 3's problem and needs a real home
// directory; it does not matter to a basename.
//
// Backslashes are folded to slashes first, so a Windows-style path the client
// passed through does not read as one long basename.
func basename(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if p == "~" {
		return ""
	}
	p = strings.TrimPrefix(p, "~/")
	// A trailing separator names a directory, and path.Base would hand back
	// the directory's own name as though a file had been named. There is no
	// file here to describe, so the answer is "we could not read a path",
	// not a label for the folder the path ends in.
	if strings.HasSuffix(p, "/") {
		return ""
	}
	return path.Base(p)
}

// labelForBase walks the table in order and returns the first row that matches.
// A base that matches no row is LabelNone: the path was read and recognised as
// nothing, which is an answer.
func labelForBase(written string) string {
	base := strings.ToLower(written)
	// "." and ".." are directory references, not filenames, and neither
	// describes a file. They belong with the empty basename rather than with
	// LabelNone, which is a real answer about a real file.
	if base == "" || base == "." || base == ".." || base == "/" {
		return LabelUnknown
	}
	for _, r := range labelTable {
		for _, e := range r.exact {
			if base == e {
				return r.label
			}
		}
		for _, p := range r.prefix {
			if strings.HasPrefix(base, p) {
				return r.label
			}
		}
		for _, s := range r.suffix {
			if strings.HasSuffix(base, s) {
				return r.label
			}
		}
		for _, e := range r.casedExact {
			if written == e {
				return r.label
			}
		}
		for _, s := range r.casedSuffix {
			if strings.HasSuffix(written, s) {
				return r.label
			}
		}
		for _, ps := range r.casedBoth {
			if strings.HasPrefix(written, ps[0]) && strings.HasSuffix(written, ps[1]) {
				return r.label
			}
		}
	}
	return LabelNone
}
