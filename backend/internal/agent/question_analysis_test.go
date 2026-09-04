package agent

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestRoutingRegressionEvaluation(t *testing.T) {
	data, err := os.ReadFile("../../../evals/routing.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var suite struct {
		Now   time.Time `yaml:"now"`
		Cases []struct {
			Question  string   `yaml:"question"`
			Route     string   `yaml:"expected_route"`
			Retrieval []string `yaml:"expected_retrieval"`
			Reason    string   `yaml:"expected_reason"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if suite.Now.IsZero() || len(suite.Cases) < 25 {
		t.Fatal("routing suite missing clock or cases")
	}
	router := NewPolicyRouterWithAnalyzer(NewQuestionAnalyzer(func() time.Time { return suite.Now }))
	for _, c := range suite.Cases {
		t.Run(c.Question, func(t *testing.T) {
			plan, err := router.Plan(context.Background(), Request{Question: c.Question})
			if err != nil {
				t.Fatal(err)
			}
			retrieval := []string{}
			if plan.Knowledge != nil {
				retrieval = append(retrieval, "knowledge")
			}
			if plan.Search != nil {
				retrieval = append(retrieval, "web")
			}
			if plan.Route != c.Route || plan.Reason != c.Reason || !reflect.DeepEqual(retrieval, c.Retrieval) {
				t.Fatalf("got route=%s reason=%s retrieval=%v; want %s %s %v", plan.Route, plan.Reason, retrieval, c.Route, c.Reason, c.Retrieval)
			}
		})
	}
}

func TestQuestionAnalysisPreservesSemantics(t *testing.T) {
	analyzer := NewQuestionAnalyzer(func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) })
	for _, c := range []struct{ original, effective string }{
		{"您好！麻烦问一下，我想请教一下今年四六级什么时候报名？", "今年四六级什么时候报名？"},
		{"你好像知道今年什么时候报名", "你好像知道今年什么时候报名"},
		{"你好不好用不重要，帮我查一下校历", "你好不好用不重要，帮我查一下校历"},
		{"我想问一下你好是什么意思", "你好是什么意思"},
		{"你好请问图书馆在哪里？", "图书馆在哪里？"},
		{"HELLO，请问今年什么时候放假？", "今年什么时候放假？"},
	} {
		t.Run(c.original, func(t *testing.T) {
			p := analyzer.Analyze(c.original)
			if p.OriginalQuestion != c.original || p.EffectiveQuestion != c.effective {
				t.Fatalf("%+v", p)
			}
			plan, err := NewPolicyRouterWithAnalyzer(analyzer).Plan(context.Background(), Request{Question: c.original})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Knowledge != nil && plan.Knowledge.Query != c.effective {
				t.Fatal("knowledge query retains wrapper")
			}
			if plan.Search != nil && plan.Search.Query != c.effective {
				t.Fatal("web query retains wrapper")
			}
		})
	}
}

func TestPolicyRouterRejectsEmptyAndCancelledInput(t *testing.T) {
	if _, err := NewPolicyRouter().Plan(context.Background(), Request{Question: " \n\t"}); err == nil {
		t.Fatal("empty input accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewPolicyRouter().Plan(ctx, Request{Question: "你好"}); err == nil {
		t.Fatal("cancel ignored")
	}
}
