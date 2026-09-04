package integration

import (
	"context"
	"testing"
)

func TestCleaningReceiptRejectsStaleOrIncompleteKnowledge(t *testing.T) {
	h := newHarness(t, &fixtureWeb{}, true)
	assertFound := func(want bool) {
		t.Helper()
		_, found, err := h.db.ResolveEvidence(context.Background(), "eval", "kb-document")
		if err != nil || found != want {
			t.Fatalf("found=%v want=%v err=%v", found, want, err)
		}
	}
	assertFound(true)
	for _, change := range []string{
		"admission_status='BLOCKED'", "parse_status='FAILED'", "pii_scan_status='NOT_SCANNED'",
		"pii_content_hash='old-hash'", "content_chars=0", "secondary_topic='other'",
		"review_status='REVIEW'", "pii_detected=true", "admission_version='old-rule'",
	} {
		h.execSQL("UPDATE knowledge.documents SET " + change + " WHERE id='document'")
		assertFound(false)
		h.execSQL(`UPDATE knowledge.documents SET admission_status='READY',parse_status='PARSED',
			pii_scan_status='CLEAR',pii_content_hash='fixture-hash',content_chars=200,
			secondary_topic='scholarship',review_status='ACCEPTED',pii_detected=false,
			admission_version='admission-v1' WHERE id='document'`)
	}
	assertFound(true)
	h.execSQL(`INSERT INTO knowledge.attachments(id,document_id,name,attachment_original_url,
		parent_page_url,rag_eligible,pii_detected,review_status)
		VALUES('attachment','document','附件','https://university.example/policy.pdf',
		'https://university.example/rules',true,false,'ACCEPTED')`)
	h.execSQL(`UPDATE knowledge.weknora_mappings SET attachment_id='attachment' WHERE weknora_knowledge_id='kb-document'`)
	assertFound(false)
	h.execSQL(`UPDATE knowledge.attachments SET admission_status='READY',parse_status='PARSED',
		pii_scan_status='CLEAR',content_hash='attachment-hash',pii_content_hash='attachment-hash',
		relation_status='RESOLVED' WHERE id='attachment'`)
	assertFound(true)
	h.execSQL(`UPDATE knowledge.attachments SET relation_status='UNRESOLVED' WHERE id='attachment'`)
	assertFound(false)
}
