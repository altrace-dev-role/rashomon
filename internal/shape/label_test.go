package shape

import (
	"encoding/json"
	"testing"
)

func pathInput(t *testing.T, field, value string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{field: value, "description": "irrelevant prose"})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestLabel covers what a path actually looks like in a tool input, which is
// the only thing this function is allowed to see: the basename, case-folded,
// against a fixed table. Every case names the convention it stands for,
// because a row that cannot be named is a row that was widened until something
// matched.
func TestLabel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tool  string
		field string
		path  string
		want  string
		why   string
	}{
		{name: "openssh private key", tool: "Read", field: "file_path", path: "~/.ssh/id_rsa", want: LabelSSHKey},
		{name: "case folded", tool: "Read", field: "file_path", path: "~/.SSH/ID_RSA", want: LabelSSHKey,
			why: "H-45: a case variant labels as the canonical spelling does"},
		{name: "public half", tool: "Read", field: "file_path", path: "/home/a/.ssh/id_ed25519.pub", want: LabelSSHKey,
			why: "the label says what the path looks like, not that it is secret"},
		{name: "suffixed key", tool: "Read", field: "file_path", path: "/home/a/.ssh/id_rsa_work", want: LabelSSHKey},
		{name: "authorized keys", tool: "Read", field: "file_path", path: "/home/a/.ssh/authorized_keys", want: LabelSSHKey},

		{name: "dotenv", tool: "Read", field: "file_path", path: "/srv/app/.env", want: LabelEnvFile},
		{name: "per environment dotenv", tool: "Read", field: "file_path", path: "/srv/app/.env.production", want: LabelEnvFile},
		{name: "direnv", tool: "Read", field: "file_path", path: "/srv/app/.envrc", want: LabelEnvFile},

		{name: "kubeconfig", tool: "Read", field: "file_path", path: "/home/a/.kube/kubeconfig", want: LabelCloudConfig},
		{name: "terraform state", tool: "Read", field: "file_path", path: "/infra/terraform.tfstate", want: LabelCloudConfig,
			why: "state holds provider secrets in cleartext"},
		{name: "gcloud adc", tool: "Read", field: "file_path",
			path: "/home/a/.config/gcloud/application_default_credentials.json", want: LabelCloudConfig,
			why: "an exact name in the cloud-config row; it matches no other row, so it says nothing about order"},

		// Row order is a stated contract, so it needs basenames that really
		// do match two rows. These three do; the gcloud name above does not,
		// and claiming it proved the ordering was the kind of assertion that
		// passes whatever the order is.
		{name: "order: cloud-config beats credential-shaped", tool: "Read", field: "file_path",
			path: "/infra/secrets.tfvars", want: LabelCloudConfig,
			why: "matches the .tfvars suffix AND the secrets. prefix; the provider's own convention wins"},
		{name: "order: credential-shaped beats certificate", tool: "Read", field: "file_path",
			path: "/vault/secrets.pem", want: LabelCredentialShaped,
			why: "matches the secrets. prefix AND the .pem suffix; a name that says what the file is beats an encoding"},

		{name: "pem", tool: "Read", field: "file_path", path: "/etc/ssl/server.pem", want: LabelCertificate},
		{name: "private key half of the same family", tool: "Read", field: "file_path", path: "/etc/ssl/server.key", want: LabelCertificate,
			why: "the vocabulary has no separate private-key value; X.509 material is one naming family"},
		{name: "pkcs12", tool: "Read", field: "file_path", path: "/etc/ssl/bundle.p12", want: LabelCertificate},
		{name: "java keystore", tool: "Read", field: "file_path", path: "/opt/app/app.jks", want: LabelCertificate},

		{name: "netrc", tool: "Read", field: "file_path", path: "/home/a/.netrc", want: LabelCredentialShaped},
		{name: "aws credentials", tool: "Read", field: "file_path", path: "/home/a/.aws/credentials", want: LabelCredentialShaped},
		{name: "npmrc", tool: "Read", field: "file_path", path: "/home/a/.npmrc", want: LabelCredentialShaped},
		{name: "secrets convention", tool: "Read", field: "file_path", path: "/k8s/secrets.yaml", want: LabelCredentialShaped},
		{name: "keepass", tool: "Read", field: "file_path", path: "/home/a/vault.kdbx", want: LabelCredentialShaped},

		{name: "ordinary source file", tool: "Read", field: "file_path", path: "/srv/app/main.go", want: LabelNone,
			why: "a path read and recognised as nothing is an answer, not unknown"},
		{name: "ordinary doc", tool: "Read", field: "file_path", path: "README.md", want: LabelNone},

		{name: "write is labelled", tool: "Write", field: "file_path", path: "/srv/.env", want: LabelEnvFile},
		{name: "edit is labelled", tool: "Edit", field: "file_path", path: "/srv/.env", want: LabelEnvFile},
		{name: "notebook path", tool: "NotebookEdit", field: "notebook_path", path: "/srv/secrets.ipynb", want: LabelCredentialShaped},

		{name: "notebook edit does not read file_path", tool: "NotebookEdit", field: "file_path", path: "~/.ssh/id_rsa", want: LabelUnknown,
			why: "each tool is read for exactly one field; the wrong field is an absent field"},
		{name: "empty path", tool: "Read", field: "file_path", path: "", want: LabelUnknown},
		{name: "bare tilde", tool: "Read", field: "file_path", path: "~", want: LabelUnknown},

		{name: "windows separators", tool: "Read", field: "file_path", path: `C:\Users\a\.ssh\id_rsa`, want: LabelSSHKey,
			why: "a backslash path must not read as one long basename"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Label(tc.tool, pathInput(t, tc.field, tc.path))
			if got != tc.want {
				t.Errorf("Label(%q, %q) = %q, want %q%s", tc.tool, tc.path, got, tc.want, because(tc.why))
			}
		})
	}
}

func because(why string) string {
	if why == "" {
		return ""
	}
	return "\n  " + why
}

// TestLabelOnlyWhereAPathIsRead is H-46's unit half: a tool this version does
// not label gets no label at all, which is not the same statement as unknown.
func TestLabelOnlyWhereAPathIsRead(t *testing.T) {
	for _, tool := range []string{"Bash", "Agent", "Task", "WebFetch", "Glob", "Grep", "MultiEdit", "mcp__x__y"} {
		t.Run(tool, func(t *testing.T) {
			if got := Label(tool, pathInput(t, "file_path", "~/.ssh/id_rsa")); got != "" {
				t.Errorf("Label(%q, ...) = %q, want the empty string: this tool names no file in version 1", tool, got)
			}
		})
	}
}

// TestLabelMalformedInput covers every way an input can fail to name a path.
// All of them are unknown, and none of them panics: the hook path recovers,
// but a function that relies on that recovery is a function whose failure mode
// is a missing record.
func TestLabelMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "empty input", raw: ``},
		{name: "not an object", raw: `"just a string"`},
		{name: "array", raw: `[1,2,3]`},
		{name: "field absent", raw: `{"description":"no path here"}`},
		{name: "field is a number", raw: `{"file_path":42}`},
		{name: "field is null", raw: `{"file_path":null}`},
		{name: "field is an object", raw: `{"file_path":{"nested":"x"}}`},
		{name: "malformed json", raw: `{"file_path":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Label("Read", json.RawMessage(tc.raw)); got != LabelUnknown {
				t.Errorf("Label(Read, %s) = %q, want %q", tc.raw, got, LabelUnknown)
			}
		})
	}
}

// TestLabelVocabularyIsClosed is H-45's closure half: whatever the table grows
// to, every value it can emit is in Labels(). A row added with a value that is
// not in the vocabulary is a record that will fail the published schema in the
// field rather than here.
func TestLabelVocabularyIsClosed(t *testing.T) {
	vocab := make(map[string]bool, len(Labels()))
	for _, l := range Labels() {
		vocab[l] = true
	}
	for _, r := range labelTable {
		if !vocab[r.label] {
			t.Errorf("table row emits %q, which is not in Labels()", r.label)
		}
	}
	// Every constant is reachable from the table or is one of the two
	// outcomes the table itself cannot produce.
	emitted := map[string]bool{LabelNone: true, LabelUnknown: true}
	for _, r := range labelTable {
		emitted[r.label] = true
	}
	for _, l := range Labels() {
		if !emitted[l] {
			t.Errorf("Labels() carries %q, which no row emits: the vocabulary and the table disagree", l)
		}
	}
}

// TestLabelIsDeterministic is H-45's determinism half at unit level. Label
// takes no key and no clock, so two installs cannot disagree; this asserts the
// property rather than the signature, so it keeps holding if the signature
// changes.
func TestLabelIsDeterministic(t *testing.T) {
	in := pathInput(t, "file_path", "~/.ssh/id_rsa")
	first := Label("Read", in)
	for i := 0; i < 100; i++ {
		if got := Label("Read", in); got != first {
			t.Fatalf("run %d returned %q after %q", i, got, first)
		}
	}
}

// TestLabelSecretPrefixDoesNotSwallowProse guards the one row whose pattern is
// a prefix over a common English word. The convention is secrets.yaml, not
// every file whose name begins with "secret".
func TestLabelSecretPrefixDoesNotSwallowProse(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/docs/secret-santa.md", want: LabelNone},
		{path: "/src/secret_handshake.go", want: LabelNone},
		{path: "/k8s/secrets.yaml", want: LabelCredentialShaped},
		{path: "/k8s/secrets.json", want: LabelCredentialShaped},
		{path: "/etc/secrets", want: LabelCredentialShaped},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := Label("Read", pathInput(t, "file_path", tc.path)); got != tc.want {
				t.Errorf("Label(Read, %q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestLabelNeverReadsACommandLine is the case the mutation sweep found
// missing: TestLabelOnlyWhereAPathIsRead passes a file_path to a Bash call,
// which a derivation that reads `command` would ignore anyway. A break that
// labels Bash from the first path-like token in its command line survived
// that test. These inputs are what a Bash call actually looks like.
func TestLabelNeverReadsACommandLine(t *testing.T) {
	for _, cmd := range []string{
		"cat ~/.ssh/id_rsa",
		"cp /etc/ssl/server.pem /tmp/x",
		"vim /srv/app/.env",
		"openssl rsa -in /etc/ssl/server.key",
	} {
		t.Run(cmd, func(t *testing.T) {
			in, err := json.Marshal(map[string]string{"command": cmd, "description": "irrelevant prose"})
			if err != nil {
				t.Fatal(err)
			}
			if got := Label("Bash", in); got != "" {
				t.Errorf("Label(Bash, %q) = %q, want the empty string: a file target guessed from a command line is a guess", cmd, got)
			}
		})
	}
}

// TestLabelEdgeCasesFromTheHunt pins the cases an edge-case sweep of the table
// turned up. Each one is a path a real tool writes or a real user has, and
// each was wrong before it was listed here.
func TestLabelEdgeCasesFromTheHunt(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
		why  string
	}{
		{path: "/infra/terraform.tfstate.backup", want: LabelCloudConfig,
			why: "terraform apply writes it every run and it holds the same cleartext secrets"},
		{path: "/infra/terraform.tfstate.bak", want: LabelCloudConfig},
		{path: "/home/u/.config/gcloud/credentials.json", want: LabelCredentialShaped,
			why: "GCP service-account keys ship under this name"},
		{path: "/app/config/credentials.yml.enc", want: LabelCredentialShaped,
			why: "Rails writes its credentials here"},

		{path: "/srv/.environment", want: LabelNone,
			why: "a bare .env prefix would swallow prose the way a bare secret prefix would"},
		{path: "/srv/.envelope", want: LabelNone},
		{path: "/srv/.env", want: LabelEnvFile},
		{path: "/srv/.env.staging", want: LabelEnvFile},
		{path: "/srv/.env-local", want: LabelEnvFile},

		{path: "/vault/secret.key", want: LabelCredentialShaped,
			why: "a name that says what the file is beats an extension that says how it is encoded"},
		{path: "/vault/credentials.pem", want: LabelCredentialShaped},
		{path: "/etc/ssl/server.key", want: LabelCertificate,
			why: "the X.509 row still owns a name that says nothing else"},
		{path: "/etc/ssl/server.pem", want: LabelCertificate},

		{path: "/docs/release.asc", want: LabelNone,
			why: "an armoured .asc is most often a detached signature, which is public by construction"},

		{path: "/srv/app/..", want: LabelUnknown,
			why: "a directory reference names no file; it belongs with the empty basename, not with none"},
		{path: "/srv/app/.", want: LabelUnknown},
		{path: "/srv/app/", want: LabelUnknown,
			why: "a trailing separator names a directory, and path.Base would hand back its name as a file"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := Label("Read", pathInput(t, "file_path", tc.path)); got != tc.want {
				t.Errorf("Label(Read, %q) = %q, want %q%s", tc.path, got, tc.want, because(tc.why))
			}
		})
	}
}
