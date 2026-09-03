package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"asku/backend/internal/evaluation"
)

func main() { os.Exit(run()) }

func run() int {
	root := flag.String("root", "..", "repository root (default: run from backend)")
	suite := flag.String("suite", "offline", "offline, integration or all")
	out := flag.String("out", "", "report directory (default: evals/reports/<UTC timestamp>)")
	race := flag.Bool("race", false, "enable Go race detector (requires a C compiler)")
	timeout := flag.Duration("timeout", 5*time.Minute, "total execution deadline")
	flag.Parse()
	if flag.NArg() != 0 || *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments or invalid timeout")
		return 2
	}
	resolvedRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *out == "" {
		*out = filepath.Join(resolvedRoot, "evals", "reports", time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	report, err := evaluation.Run(ctx, evaluation.Options{Root: resolvedRoot, Suite: *suite, Output: *out, Race: *race})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("engineering=%s data=%s passed=%d failed=%d skipped=%d blocked_data=%d\nReports: %s\n", report.EngineeringGate, report.DataGate,
		report.Summary[evaluation.Passed], report.Summary[evaluation.Failed], report.Summary[evaluation.Skipped], report.Summary[evaluation.BlockedData], *out)
	if report.EngineeringGate != evaluation.Passed {
		return 1
	}
	return 0
}
