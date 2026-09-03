package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteReport(directory string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "report.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# AskU engineering evaluation\n\nEngineering gate: **%s**. Data gate: **%s**.\n\n", report.EngineeringGate, report.DataGate)
	fmt.Fprintf(&md, "Revision: `%s`; dirty: `%t`; suite: `%s`; race: `%t`.\n\nStarted: %s; finished: %s.\n\n", report.GitCommit, report.WorkingTreeDirty, report.Suite, report.Race, report.StartedAt.Format("2006-01-02T15:04:05Z"), report.FinishedAt.Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&md, "Fixtures: `%s`; knowledge: `%s`. All provider evidence is synthetic; this report does not measure campus answer accuracy.\n\n", report.FixtureVersion, report.KnowledgeVersion)
	fmt.Fprintf(&md, "Source SHA-256: `%s`. Fixture school: `%s`; pending data school: `%s`. Configuration and input hashes are recorded in report.json.\n\n", report.SourceSHA256, cell(report.FixtureSchool.ID), cell(report.SchoolID))
	fmt.Fprintf(&md, "Passed: %d; failed: %d; skipped: %d; blocked_data: %d. Blocked and skipped checks are never counted as passed.\n\n", report.Summary[Passed], report.Summary[Failed], report.Summary[Skipped], report.Summary[BlockedData])
	md.WriteString("| Check | Suite | Status | Duration (ms) | Detail |\n| --- | --- | --- | ---: | --- |\n")
	for _, result := range report.Results {
		detail := result.Description
		if result.Reason != "" {
			detail += "; " + result.Reason
		}
		fmt.Fprintf(&md, "| %s | %s | %s | %d | %s |\n", cell(result.ID), cell(result.Suite), result.Status, result.DurationMS, cell(detail))
	}
	if len(report.Errors) > 0 {
		md.WriteString("\n## Execution errors\n\n")
		for _, err := range report.Errors {
			fmt.Fprintf(&md, "- %s\n", cell(err))
		}
	}
	md.WriteString("\n## Pending data review\n\nSee `manualReview` and `reviewDimensions` in report.json for the original question, authority, freshness and rejection criteria. Automatic citation presence does not prove that evidence supports a conclusion.\n")
	return os.WriteFile(filepath.Join(directory, "report.md"), []byte(md.String()), 0644)
}

func cell(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "&#124;", "\n", " ", "\r", " ").Replace(s)
}
