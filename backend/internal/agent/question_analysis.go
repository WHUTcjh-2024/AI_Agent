package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type FreshnessLevel string

const (
	FreshnessStable     FreshnessLevel = "stable"
	FreshnessHistorical FreshnessLevel = "historical"
	FreshnessCurrent    FreshnessLevel = "current"
)

// QuestionProfile separates the original message from the retrieval query.
// Analysis has no dependency on providers, persistence or school configuration.
type QuestionProfile struct {
	OriginalQuestion  string
	EffectiveQuestion string
	PureSocial        bool
	ProductIntro      bool
	IntegrationProbe  bool
	Freshness         FreshnessLevel
	Reason            string
}

type QuestionAnalyzer struct{ now func() time.Time }

func NewQuestionAnalyzer(now func() time.Time) *QuestionAnalyzer {
	if now == nil {
		now = time.Now
	}
	return &QuestionAnalyzer{now: now}
}

var politePrefixes = []string{
	"能不能帮我看看", "麻烦帮我看看", "请帮我查一下", "我想请教一下", "想请教一下",
	"我想问一下", "麻烦问一下", "想问一下", "请问",
}
var socialPrefixes = []string{"hello", "您好", "你好", "哈喽", "在吗", "hi", "嗨"}

func wrapperSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("，,。.!！?？:：;；、", r)
}

// Greetings require a boundary, a standalone particle, or another recognized
// wrapper. Ambiguous text (你好像/你好不好用/helloWorld) is preserved.
func stripGreeting(question string) (string, bool) {
	for _, prefix := range socialPrefixes {
		if !strings.HasPrefix(strings.ToLower(question), prefix) {
			continue
		}
		rest := question[len(prefix):]
		if rest == "" {
			return rest, true
		}
		r, size := utf8.DecodeRuneInString(rest)
		if strings.ContainsRune("呀啊哦哟", r) {
			rest = rest[size:]
		}
		if rest == "" {
			return rest, true
		}
		r, _ = utf8.DecodeRuneInString(rest)
		if wrapperSeparator(r) {
			return strings.TrimLeftFunc(rest, wrapperSeparator), true
		}
		for _, polite := range politePrefixes {
			if strings.HasPrefix(rest, polite) {
				return rest, true
			}
		}
	}
	return question, false
}

func (a *QuestionAnalyzer) Analyze(original string) QuestionProfile {
	q := strings.Join(strings.Fields(original), " ")
	social := false
	for {
		if rest, ok := stripGreeting(q); ok {
			q, social = rest, true
			continue
		}
		stripped := false
		for _, prefix := range politePrefixes {
			if strings.HasPrefix(q, prefix) {
				q = strings.TrimLeftFunc(q[len(prefix):], wrapperSeparator)
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	p := QuestionProfile{OriginalQuestion: original, EffectiveQuestion: q, PureSocial: q == "" && social}
	if p.PureSocial {
		p.Freshness, p.Reason = FreshnessStable, "pure_social"
		return p
	}
	// Politeness alone is not a greeting; adapters must receive a nonempty query.
	if q == "" {
		q = strings.TrimSpace(original)
		p.EffectiveQuestion = q
	}
	intro := strings.ToLower(strings.TrimFunc(q, wrapperSeparator))
	intro = strings.ReplaceAll(intro, " ", "")
	switch intro {
	case "你是谁", "你能做什么", "介绍一下你自己", "介绍一下自己", "介绍一下asku", "asku是什么", "asku能干什么", "asku能做什么":
		p.ProductIntro, p.Freshness, p.Reason = true, FreshnessStable, "product_introduction"
		return p
	}
	p.IntegrationProbe = intro == "官网搜索测试" || intro == "web-search"
	p.Freshness, p.Reason = classifyFreshness(q, a.now().Year())
	if p.IntegrationProbe {
		p.Reason = "integration_probe"
	}
	return p
}

var explicitYear = regexp.MustCompile(`(?:^|[^0-9])((?:19|20|21)[0-9]{2})(?:\s*年|\s*[-—–/]\s*((?:19|20|21)[0-9]{2})\s*(?:学年|年))`)
var freshnessRules = []struct {
	reason  string
	pattern *regexp.Regexp
}{
	{"freshness_status_question", regexp.MustCompile(`还能不能|还能|现在能不能|开始了吗|结束了吗|开放了吗|报名了吗|是否有效|还有效吗|是否已经|有没有开始`)},
	{"freshness_current_marker", regexp.MustCompile(`现在|目前|当前|最新|最近|今年|本年度|本学年|本学期|这学期|下学期|本轮|这次|近期|今天|明天|本周|下周|本月|明年`)},
}
var scheduleQuestion = regexp.MustCompile(`什么时候|何时|几号|几点|时间|日期|截止|开始|结束|放假|开学|开门|关门`)
var dynamicInformation = regexp.MustCompile(`校历|报名|考试|选课|开放|最新政策|本年度通知`)
var stableBackground = regexp.MustCompile(`是什么|在哪里|在哪儿|什么条件|哪些.{0,4}条件|基本条件|一般怎么|如何理解|什么意思`)

func classifyFreshness(q string, year int) (FreshnessLevel, string) {
	for _, rule := range freshnessRules {
		if rule.pattern.MatchString(q) {
			return FreshnessCurrent, rule.reason
		}
	}
	// Current-state signals above win even when the cited policy is historical.
	years := explicitYear.FindAllStringSubmatch(q, -1)
	for _, match := range years {
		for _, value := range match[1:] {
			if n, err := strconv.Atoi(value); err == nil && n >= year {
				return FreshnessCurrent, "freshness_current_year"
			}
		}
	}
	if len(years) > 0 {
		return FreshnessHistorical, "historical_explicit_year"
	}
	if scheduleQuestion.MatchString(q) {
		return FreshnessCurrent, "freshness_schedule_question"
	}
	if dynamicInformation.MatchString(q) && !stableBackground.MatchString(q) {
		return FreshnessCurrent, "freshness_dynamic_information"
	}
	return FreshnessStable, "stable_campus_knowledge"
}
