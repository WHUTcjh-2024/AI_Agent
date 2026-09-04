package evaluation

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"asku/backend/internal/school"
)

const (
	Passed      = "passed"
	Failed      = "failed"
	Skipped     = "skipped"
	BlockedData = "blocked_data"
)

type Result struct {
	Case
	Status     string          `json:"status"`
	Reason     string          `json:"reason,omitempty"`
	DurationMS int64           `json:"durationMs"`
	Review     *GoldenQuestion `json:"manualReview,omitempty"`
}

type Report struct {
	Version          int               `json:"version"`
	StartedAt        time.Time         `json:"startedAt"`
	FinishedAt       time.Time         `json:"finishedAt"`
	Suite            string            `json:"suite"`
	GitCommit        string            `json:"gitCommit"`
	WorkingTreeDirty bool              `json:"workingTreeDirty"`
	GoVersion        string            `json:"goVersion"`
	Race             bool              `json:"race"`
	FixtureVersion   string            `json:"fixtureVersion"`
	Providers        map[string]string `json:"providers"`
	InputSHA256      map[string]string `json:"inputSha256"`
	SourceSHA256     string            `json:"sourceSha256"`
	FixtureSchool    school.Context    `json:"fixtureSchool"`
	SchoolID         string            `json:"schoolId"`
	KnowledgeVersion string            `json:"knowledgeVersion"`
	ReviewDimensions map[string]string `json:"reviewDimensions"`
	Summary          map[string]int    `json:"summary"`
	EngineeringGate  string            `json:"engineeringGate"`
	DataGate         string            `json:"dataGate"`
	Results          []Result          `json:"results"`
	Errors           []string          `json:"errors"`
}

type Options struct {
	Root   string
	Suite  string
	Output string
	Race   bool
}

type testEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
}

// Run uses Go's structured test events as the execution protocol. A missing,
// skipped or crashed selected test cannot silently count as a successful check.
func Run(ctx context.Context, opts Options) (Report, error) {
	r := Report{Version: 1, StartedAt: time.Now().UTC(), Suite: opts.Suite, GoVersion: runtime.Version(), Race: opts.Race,
		FixtureVersion: "engineering-v1", KnowledgeVersion: "fixture-v1 (isolated; no admitted campus data)",
		Providers:   map[string]string{"llm": "local mock/stubs", "web": "local fixtures/HTTP stubs", "knowledge": "disabled or isolated fixtures"},
		InputSHA256: map[string]string{}, Summary: map[string]int{Passed: 0, Failed: 0, Skipped: 0, BlockedData: 0},
		DataGate: BlockedData, Results: []Result{}, Errors: []string{},
	}
	if opts.Suite != "offline" && opts.Suite != "integration" && opts.Suite != "all" {
		return r, fmt.Errorf("suite must be offline, integration or all")
	}
	manifestPath := filepath.Join(opts.Root, "evals", "engineering.yaml")
	goldenPath := filepath.Join(opts.Root, "evals", "golden-questions.yaml")
	manifest, golden, err := Load(manifestPath, goldenPath)
	if err != nil {
		return r, err
	}
	r.SchoolID, r.ReviewDimensions = golden.SchoolID, golden.Dimensions
	fixtureSchool, err := school.Load(filepath.Join(opts.Root, "evals", "fixtures", "school.yaml"))
	if err != nil {
		return r, err
	}
	r.FixtureSchool = fixtureSchool.Current()
	r.KnowledgeVersion = r.FixtureSchool.KnowledgeVersion
	if r.SourceSHA256, err = sourceDigest(filepath.Join(opts.Root, "backend")); err != nil {
		return r, err
	}
	for _, name := range []string{"evals/engineering.yaml", "evals/golden-questions.yaml", "evals/fixtures/school.yaml", "evals/routing.yaml"} {
		data, err := os.ReadFile(filepath.Join(opts.Root, filepath.FromSlash(name)))
		if err != nil {
			return r, err
		}
		r.InputSHA256[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = opts.Root
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if r.GitCommit, err = git("rev-parse", "HEAD"); err != nil {
		return r, fmt.Errorf("read code revision: %w", err)
	}
	status, err := git("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return r, fmt.Errorf("read working tree: %w", err)
	}
	r.WorkingTreeDirty = status != ""
	selected := []Case{}
	for _, c := range manifest.Cases {
		if opts.Suite == "all" || opts.Suite == c.Suite {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 {
		return r, fmt.Errorf("selected suite contains no checks")
	}
	if err := os.MkdirAll(opts.Output, 0755); err != nil {
		return r, err
	}
	logFile, err := os.Create(filepath.Join(opts.Output, "go-test.jsonl"))
	if err != nil {
		return r, err
	}
	defer logFile.Close()
	states, executionErr := execute(ctx, opts, selected, logFile)
	if executionErr != nil {
		r.Errors = append(r.Errors, executionErr.Error())
	}
	for _, c := range manifest.Cases {
		result := Result{Case: c, Status: Skipped, Reason: "suite not selected"}
		if opts.Suite == "all" || opts.Suite == c.Suite {
			key := "asku/backend/" + strings.TrimPrefix(c.Package, "./") + "/" + c.Test
			result = resultFromEvent(c, states[key])
		}
		r.Results = append(r.Results, result)
	}
	for _, q := range golden.Questions {
		r.Results = append(r.Results, Result{Case: Case{ID: q.ID, Suite: "data", Description: q.Question}, Status: BlockedData,
			Reason: "awaiting admitted data, real provider run and human citation review; fixtures cannot establish answer accuracy", Review: &q})
	}
	r.EngineeringGate = Passed
	for _, result := range r.Results {
		r.Summary[result.Status]++
		if result.Status == Failed || (result.Status == Skipped && result.Reason != "suite not selected") {
			r.EngineeringGate = Failed
		}
	}
	if executionErr != nil {
		r.EngineeringGate = Failed
	}
	r.FinishedAt = time.Now().UTC()
	if err := WriteReport(opts.Output, r); err != nil {
		return r, err
	}
	return r, nil
}

// Include uncommitted source bytes in the identity of a local evaluation.
func sourceDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".sql" && entry.Name() != "go.mod" && entry.Name() != "go.sum" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
		_, _ = hash.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func resultFromEvent(c Case, event testEvent) Result {
	r := Result{Case: c, Status: Failed, Reason: "test did not produce a terminal result (missing, interrupted or build failed)", DurationMS: int64(event.Elapsed * 1000)}
	switch event.Action {
	case "pass":
		r.Status, r.Reason = Passed, ""
	case "fail":
		r.Reason = "test assertions or execution failed; see go-test.jsonl"
	case "skip":
		r.Status, r.Reason = Skipped, "selected test skipped; engineering gate fails"
	}
	return r
}

func execute(ctx context.Context, opts Options, cases []Case, log io.Writer) (map[string]testEvent, error) {
	packages, tests := map[string]bool{}, map[string]bool{}
	for _, c := range cases {
		packages[c.Package], tests[c.Test] = true, true
	}
	keys := func(m map[string]bool) []string {
		result := make([]string, 0, len(m))
		for k := range m {
			result = append(result, k)
		}
		sort.Strings(result)
		return result
	}
	args := []string{"test", "-json", "-count=1", "-timeout=120s", "-run", "^(" + strings.Join(keys(tests), "|") + ")$"}
	if opts.Race {
		args = append(args, "-race")
	}
	args = append(args, keys(packages)...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = filepath.Join(opts.Root, "backend")
	cmd.Env = append(os.Environ(), "ASKU_EVAL_INTEGRATION=1", "ASKU_EVAL_ROOT="+opts.Root)
	stderr := &limitedBuffer{limit: 8192}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	states, parseErr := readEvents(io.TeeReader(stdout, log))
	if parseErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		return states, parseErr
	}
	if waitErr != nil {
		return states, fmt.Errorf("go test failed: %w; %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return states, nil
}

func readEvents(reader io.Reader) (map[string]testEvent, error) {
	states := map[string]testEvent{}
	skippedChildren := map[string]bool{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return states, fmt.Errorf("invalid go test event: %w", err)
		}
		if event.Test != "" && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			states[event.Package+"/"+event.Test] = event
			if root, _, child := strings.Cut(event.Test, "/"); child && event.Action == "skip" {
				skippedChildren[event.Package+"/"+root] = true
			}
		}
	}
	for key := range skippedChildren {
		if event := states[key]; event.Action == "pass" {
			event.Action = "skip"
			states[key] = event
		}
	}
	return states, scanner.Err()
}

type limitedBuffer struct {
	strings.Builder
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := b.limit - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Builder.Write(p)
	}
	return n, nil
}
