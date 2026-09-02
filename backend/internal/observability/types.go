package observability

import "time"

// Window is a half-open reporting interval: [From, To).
type Window struct {
	SchoolID string
	From     time.Time
	To       time.Time
	TimeZone string
}

type Overview struct {
	GeneratedAt  time.Time          `json:"generatedAt"`
	Window       ReportWindow       `json:"window"`
	Users        UserMetrics        `json:"users"`
	Engagement   EngagementMetrics  `json:"engagement"`
	Quality      QualityMetrics     `json:"quality"`
	Performance  PerformanceMetrics `json:"performance"`
	Cost         CostMetrics        `json:"cost"`
	Routes       []CountMetric      `json:"routes"`
	ErrorCodes   []CountMetric      `json:"errorCodes"`
	TopQuestions []QuestionMetric   `json:"topQuestions"`
	Daily        []DailyMetric      `json:"daily"`
}

type ReportWindow struct {
	SchoolID string    `json:"schoolId"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	TimeZone string    `json:"timeZone"`
}

type UserMetrics struct {
	TotalRegistered int64   `json:"totalRegistered"`
	NewRegistered   int64   `json:"newRegistered"`
	ActiveUsers     int64   `json:"activeUsers"`
	D1Eligible      int64   `json:"d1Eligible"`
	D1Retained      int64   `json:"d1Retained"`
	D1RetentionRate float64 `json:"d1RetentionRate"`
	D7Eligible      int64   `json:"d7Eligible"`
	D7Retained      int64   `json:"d7Retained"`
	D7RetentionRate float64 `json:"d7RetentionRate"`
}

type EngagementMetrics struct {
	Questions              int64   `json:"questions"`
	ActiveSessions         int64   `json:"activeSessions"`
	QuestionsPerActiveUser float64 `json:"questionsPerActiveUser"`
	AverageSessionTurns    float64 `json:"averageSessionTurns"`
}

type QualityMetrics struct {
	Runs            int64   `json:"runs"`
	CompletedRuns   int64   `json:"completedRuns"`
	FailedRuns      int64   `json:"failedRuns"`
	CancelledRuns   int64   `json:"cancelledRuns"`
	SuccessRate     float64 `json:"successRate"`
	NoSourceAnswers int64   `json:"noSourceAnswers"`
	NoSourceRate    float64 `json:"noSourceRate"`
	Unhelpful       int64   `json:"unhelpfulFeedback"`
}

type PerformanceMetrics struct {
	AverageLatencyMS float64 `json:"averageLatencyMs"`
	P50LatencyMS     float64 `json:"p50LatencyMs"`
	P95LatencyMS     float64 `json:"p95LatencyMs"`
	AverageTTFTMS    float64 `json:"averageTtftMs"`
	P95TTFTMS        float64 `json:"p95TtftMs"`
}

type CostMetrics struct {
	InputTokens                      int64   `json:"inputTokens"`
	OutputTokens                     int64   `json:"outputTokens"`
	EstimatedCostMicroRMB            int64   `json:"estimatedCostMicroRmb"`
	EstimatedCostPerQuestionMicroRMB float64 `json:"estimatedCostPerQuestionMicroRmb"`
	CacheHits                        int64   `json:"cacheHits"`
	CacheHitRate                     float64 `json:"cacheHitRate"`
}

type CountMetric struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type QuestionMetric struct {
	Question string `json:"question"`
	Count    int64  `json:"count"`
}

type DailyMetric struct {
	Date                  string `json:"date"`
	NewUsers              int64  `json:"newUsers"`
	ActiveUsers           int64  `json:"activeUsers"`
	Questions             int64  `json:"questions"`
	Runs                  int64  `json:"runs"`
	CompletedRuns         int64  `json:"completedRuns"`
	FailedRuns            int64  `json:"failedRuns"`
	EstimatedCostMicroRMB int64  `json:"estimatedCostMicroRmb"`
}

func (o *Overview) Finalize(window Window, now time.Time) {
	o.GeneratedAt = now.UTC()
	o.Window = ReportWindow{SchoolID: window.SchoolID, From: window.From, To: window.To, TimeZone: window.TimeZone}
	o.Users.D1RetentionRate = ratio(o.Users.D1Retained, o.Users.D1Eligible)
	o.Users.D7RetentionRate = ratio(o.Users.D7Retained, o.Users.D7Eligible)
	o.Engagement.QuestionsPerActiveUser = ratio(o.Engagement.Questions, o.Users.ActiveUsers)
	o.Engagement.AverageSessionTurns = ratio(o.Engagement.Questions, o.Engagement.ActiveSessions)
	o.Quality.SuccessRate = ratio(o.Quality.CompletedRuns, o.Quality.Runs)
	o.Quality.NoSourceRate = ratio(o.Quality.NoSourceAnswers, o.Quality.CompletedRuns)
	o.Cost.EstimatedCostPerQuestionMicroRMB = ratio(o.Cost.EstimatedCostMicroRMB, o.Engagement.Questions)
	o.Cost.CacheHitRate = ratio(o.Cost.CacheHits, o.Quality.Runs)
	if o.Routes == nil {
		o.Routes = []CountMetric{}
	}
	if o.ErrorCodes == nil {
		o.ErrorCodes = []CountMetric{}
	}
	if o.TopQuestions == nil {
		o.TopQuestions = []QuestionMetric{}
	}
	if o.Daily == nil {
		o.Daily = []DailyMetric{}
	}
}

func ratio(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
