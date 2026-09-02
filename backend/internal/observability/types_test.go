package observability

import (
	"testing"
	"time"
)

func TestOverviewFinalizeCalculatesRatesAndUsesEmptyArrays(t *testing.T) {
	report := Overview{
		Users:      UserMetrics{ActiveUsers: 2, D1Eligible: 4, D1Retained: 1},
		Engagement: EngagementMetrics{Questions: 6, ActiveSessions: 3},
		Quality:    QualityMetrics{Runs: 8, CompletedRuns: 4, NoSourceAnswers: 1},
		Cost:       CostMetrics{EstimatedCostMicroRMB: 12, CacheHits: 2},
	}
	window := Window{SchoolID: "whut", From: time.Unix(0, 0), To: time.Unix(3600, 0), TimeZone: "Asia/Shanghai"}
	report.Finalize(window, time.Unix(10, 0))
	if report.Users.D1RetentionRate != 0.25 || report.Engagement.QuestionsPerActiveUser != 3 || report.Engagement.AverageSessionTurns != 2 {
		t.Fatalf("unexpected user or engagement rates: %#v", report)
	}
	if report.Quality.SuccessRate != 0.5 || report.Quality.NoSourceRate != 0.25 || report.Cost.CacheHitRate != 0.25 {
		t.Fatalf("unexpected quality or cache rates: %#v", report)
	}
	if report.Routes == nil || report.ErrorCodes == nil || report.TopQuestions == nil || report.Daily == nil {
		t.Fatal("JSON collection fields must never be null")
	}
}
