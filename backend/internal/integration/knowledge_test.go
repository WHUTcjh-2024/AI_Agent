package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"asku/backend/internal/agent"
	"asku/backend/internal/domain"
	"asku/backend/internal/school"
)

func TestAPIKnowledgeCache(t *testing.T) {
	h := newHarness(t, nil, true)
	session := h.session()
	question := "奖学金怎么评？"
	first := h.send(session.ID, question)
	events := h.events(first.ID, 0, false)
	terminal(t, events, "run.completed")
	assertRoute(t, events, "knowledge")
	message := finalMessage(t, events)
	if len(message.Citations) != 1 || message.Citations[0].AskUDocumentID != "document" || message.Citations[0].WeKnoraKnowledgeID != "kb-document" {
		t.Fatal("real Catalog metadata was not used")
	}
	if strings.Contains(message.Content, "/private/") {
		t.Fatal("internal path leaked into answer")
	}
	second := h.send(session.ID, question)
	cached := h.events(second.ID, 0, false)
	terminal(t, cached, "run.completed")
	assertRoute(t, cached, "cache")
	if finalMessage(t, cached).Content != message.Content || h.knowledge.calls.Load() != 1 || h.web.calls.Load() != 0 || h.count("SELECT count(*) FROM usage_records") != 1 {
		t.Fatal("cache hit did not bypass external calls")
	}
	// Exercise version invalidation against the same real Redis entries.
	root := os.Getenv("ASKU_EVAL_ROOT")
	if root == "" {
		root = filepath.Join("..", "..", "..")
	}
	data, err := os.ReadFile(filepath.Join(root, "evals", "fixtures", "school.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "school.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "knowledge_version: fixture-v1", "knowledge_version: fixture-v2", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := school.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	versioned, err := agent.NewVersionedAnswerCache(h.cache, registry, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit, err := versioned.Lookup(t.Context(), "eval", question); err != nil || hit {
		t.Fatal("new knowledge version reused stale answer")
	}
}

func TestCatalogAdmission(t *testing.T) {
	h := newHarness(t, nil, true)
	reset := func() {
		h.execSQL(`UPDATE knowledge.sources SET active=true`)
		h.execSQL(`UPDATE knowledge.documents SET rag_eligible=true,pii_detected=false,review_status='ACCEPTED'`)
		h.execSQL(`UPDATE knowledge.weknora_mappings SET import_status='IMPORTED',attachment_id=NULL`)
	}
	for _, tc := range []struct{ name, sql string }{
		{"review", `UPDATE knowledge.documents SET review_status='REVIEW'`},
		{"uncertain", `UPDATE knowledge.documents SET review_status='UNCERTAIN'`},
		{"pii", `UPDATE knowledge.documents SET pii_detected=true`},
		{"ineligible", `UPDATE knowledge.documents SET rag_eligible=false`},
		{"inactive-source", `UPDATE knowledge.sources SET active=false`},
		{"pending-import", `UPDATE knowledge.weknora_mappings SET import_status='PENDING'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			h.execSQL(tc.sql)
			if _, found, err := h.db.ResolveEvidence(t.Context(), "eval", "kb-document"); err != nil || found {
				t.Fatalf("admission failed: found=%t err=%v", found, err)
			}
		})
	}
	reset()
	for _, tc := range []struct{ school, id string }{{"eval", "unknown"}, {"another-school", "kb-document"}} {
		if _, found, err := h.db.ResolveEvidence(t.Context(), tc.school, tc.id); err != nil || found {
			t.Fatal("missing mapping or wrong school admitted")
		}
	}
	metadata, found, err := h.db.ResolveEvidence(t.Context(), "eval", "kb-document")
	if err != nil || !found || metadata.AskUDocumentID != "document" {
		t.Fatal("accepted document did not pass")
	}
	encoded, err := json.Marshal(metadata)
	if err != nil || strings.Contains(string(encoded), "/private/") {
		t.Fatal("Catalog exposed storage paths")
	}
	h.execSQL(`INSERT INTO knowledge.attachments(id,document_id,name,attachment_original_url,rag_eligible,pii_detected,review_status)
		VALUES('attachment','document','Fixture PDF','https://university.example/fixture.pdf',true,false,'REVIEW')`)
	h.execSQL(`UPDATE knowledge.weknora_mappings SET attachment_id='attachment'`)
	if _, found, err := h.db.ResolveEvidence(t.Context(), "eval", "kb-document"); err != nil || found {
		t.Fatal("unreviewed attachment admitted")
	}
	h.execSQL(`UPDATE knowledge.attachments SET review_status='ACCEPTED',pii_detected=true`)
	if _, found, err := h.db.ResolveEvidence(t.Context(), "eval", "kb-document"); err != nil || found {
		t.Fatal("PII attachment admitted")
	}
	h.execSQL(`UPDATE knowledge.attachments SET pii_detected=false`)
	if metadata, found, err := h.db.ResolveEvidence(t.Context(), "eval", "kb-document"); err != nil || !found || len(metadata.Attachments) != 1 {
		t.Fatal("accepted attachment missing")
	}
}

func TestInterruptedRunRecovery(t *testing.T) {
	h := newHarness(t, nil, false)
	session := h.session()
	_, run, err := h.db.CreateUserMessageAndRun(t.Context(), h.tokens.User.ID, domain.Message{
		ID: "eval-" + uniqueID(), SessionID: session.ID, Role: "user", Content: "中断场景合成问题", Status: "completed", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.db.UpdateRunStatus(t.Context(), run.ID, "GENERATING", ""); err != nil {
		t.Fatal(err)
	}
	if count, err := h.db.RecoverInterruptedRuns(t.Context()); err != nil || count != 1 {
		t.Fatalf("recovery count=%d err=%v", count, err)
	}
	if count, err := h.db.RecoverInterruptedRuns(context.Background()); err != nil || count != 0 {
		t.Fatal("repeated recovery added another terminal event")
	}
	events := h.events(run.ID, 0, false)
	terminal(t, events, "run.failed")
	failure := payload[struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}](t, events, "run.failed")
	if failure.Code != "server_restarted" || !failure.Retryable {
		t.Fatal("restart recovery lost failure reason")
	}
	h.status(run.ID, "FAILED")
}
