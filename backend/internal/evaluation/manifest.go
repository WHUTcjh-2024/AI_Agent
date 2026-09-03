// Package evaluation runs named engineering checks without treating fixture
// results as campus answer accuracy. It never calls a paid provider.
package evaluation

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Case struct {
	ID          string `yaml:"id" json:"id"`
	Suite       string `yaml:"suite" json:"suite"`
	Description string `yaml:"description" json:"description"`
	Package     string `yaml:"package" json:"package,omitempty"`
	Test        string `yaml:"test" json:"test,omitempty"`
}

type Manifest struct {
	Version int    `yaml:"version"`
	Cases   []Case `yaml:"cases"`
}

type Golden struct {
	Version    int               `yaml:"version"`
	SchoolID   string            `yaml:"school_id"`
	Dimensions map[string]string `yaml:"dimensions"`
	Questions  []GoldenQuestion  `yaml:"questions"`
}

type GoldenQuestion struct {
	ID                string   `yaml:"id" json:"id"`
	Question          string   `yaml:"question" json:"question"`
	MustHaveCitation  bool     `yaml:"must_have_citation" json:"mustHaveCitation"`
	ExpectedAuthority []string `yaml:"expected_authority" json:"expectedAuthority,omitempty"`
	RejectIf          []string `yaml:"reject_if" json:"rejectIf,omitempty"`
	FreshnessRequired bool     `yaml:"freshness_required" json:"freshnessRequired"`
	ExpectedBehavior  string   `yaml:"expected_behavior" json:"expectedBehavior,omitempty"`
}

var (
	caseIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	packagePattern = regexp.MustCompile(`^\./internal/[a-z][a-z0-9_]*$`)
	testPattern    = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)
)

func Load(manifestPath, goldenPath string) (Manifest, Golden, error) {
	var manifest Manifest
	var golden Golden
	if err := readYAML(manifestPath, &manifest); err != nil {
		return manifest, golden, err
	}
	if err := readYAML(goldenPath, &golden); err != nil {
		return manifest, golden, err
	}
	if manifest.Version != 1 || golden.Version != 1 || len(manifest.Cases) == 0 || len(golden.Questions) == 0 || golden.SchoolID == "" || len(golden.Dimensions) == 0 {
		return manifest, golden, fmt.Errorf("expected version 1 manifests with cases, questions, school and review dimensions")
	}
	ids, targets := map[string]bool{}, map[string]bool{}
	for _, c := range manifest.Cases {
		if !caseIDPattern.MatchString(c.ID) || ids[c.ID] || c.Description == "" || (c.Suite != "offline" && c.Suite != "integration") || !packagePattern.MatchString(c.Package) || !testPattern.MatchString(c.Test) {
			return manifest, golden, fmt.Errorf("invalid or duplicate case %q", c.ID)
		}
		key := c.Package + "/" + c.Test
		if targets[key] {
			return manifest, golden, fmt.Errorf("duplicate test target %q", key)
		}
		ids[c.ID], targets[key] = true, true
	}
	for _, q := range golden.Questions {
		if !caseIDPattern.MatchString(q.ID) || ids[q.ID] || q.Question == "" {
			return manifest, golden, fmt.Errorf("invalid or duplicate golden question %q", q.ID)
		}
		ids[q.ID] = true
	}
	return manifest, golden, nil
}

func readYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s must contain exactly one YAML document", path)
	}
	return nil
}
