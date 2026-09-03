package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Execute real child go-test processes: this checks the gate, not just the
// formatter. Renamed tests, assertion failures and skips must never turn green.
func TestRunGateAndDataAccounting(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("backend/go.mod", "module asku/backend\n\ngo 1.26.0\n")
	write("backend/internal/probe/probe_test.go", `package probe
import "testing"
func TestPass(t *testing.T) {}
func TestFailure(t *testing.T) { t.Fatal("deliberate assertion failure") }
func TestSkip(t *testing.T) { t.Skip("deliberate skip") }
func TestSubSkip(t *testing.T) { t.Run("pending", func(t *testing.T) { t.Skip("not verified") }) }
`)
	write("evals/fixtures/school.yaml", "school_id: eval\nschool_name: Fixture\nallowed_domains: [university.example]\nknowledge_version: fixture-v1\n")
	write("evals/golden-questions.yaml", "version: 1\nschool_id: eval\ndimensions:\n  correctness: human review\nquestions:\n  - id: pending\n    question: unverified campus fact\n    must_have_citation: true\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
	}
	git("init", "--quiet")
	git("add", ".")
	git("-c", "user.name=Evaluation Test", "-c", "user.email=eval@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
	for _, tc := range []struct{ name, status, gate string }{
		{"TestPass", Passed, Passed}, {"TestFailure", Failed, Failed}, {"TestMissing", Failed, Failed}, {"TestSkip", Skipped, Failed},
		{"TestSubSkip", Skipped, Failed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write("evals/engineering.yaml", "version: 1\ncases:\n  - id: probe\n    suite: offline\n    package: ./internal/probe\n    test: "+tc.name+"\n    description: gate probe\n")
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			output := filepath.Join(t.TempDir(), "report")
			report, err := Run(ctx, Options{Root: root, Suite: "offline", Output: output})
			if err != nil {
				t.Fatal(err)
			}
			if report.EngineeringGate != tc.gate || report.Results[0].Status != tc.status || report.Summary[BlockedData] != 1 || report.DataGate != BlockedData {
				t.Fatalf("incorrect gate/accounting: %#v", report)
			}
			if tc.status != Passed && report.Summary[Passed] != 0 {
				t.Fatal("non-pass counted as passing")
			}
			data, err := os.ReadFile(filepath.Join(output, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var persisted Report
			if err := json.Unmarshal(data, &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Results[1].Review == nil || !persisted.Results[1].Review.MustHaveCitation {
				t.Fatal("manual review requirements lost")
			}
			if _, err := os.Stat(filepath.Join(output, "report.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEventParserRejectsInvalidOrTruncatedJSON(t *testing.T) {
	for _, input := range []string{"not json\n", "{\"Action\":\"pass\""} {
		if _, err := readEvents(strings.NewReader(input)); err == nil {
			t.Fatal("malformed test output accepted")
		}
	}
	events, err := readEvents(strings.NewReader("{\"Action\":\"pass\",\"Package\":\"p\",\"Test\":\"TestA\",\"Elapsed\":0.1}\n"))
	if err != nil || resultFromEvent(Case{}, events["p/TestA"]).DurationMS != 100 {
		t.Fatal("test timing/status lost")
	}
}

func TestManifestRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	for _, input := range []string{"version: 1\nunknown: true\n", "version: 1\n---\nversion: 1\n"} {
		path := filepath.Join(t.TempDir(), "manifest.yaml")
		if err := os.WriteFile(path, []byte(input), 0644); err != nil {
			t.Fatal(err)
		}
		var manifest Manifest
		if err := readYAML(path, &manifest); err == nil {
			t.Fatal("invalid schema accepted")
		}
	}
}
