package citation

import "testing"

func TestBuildRejectsInternalPathsAndAssignsConsecutiveIndices(t *testing.T) {
	citations := Build([]Candidate{
		{SourceID: "bad", Title: "内部文件", EvidenceText: "内容", AttachmentURL: "/data/whut/a.pdf"},
		{SourceID: "good", ChunkID: "c1", Title: "正式通知", EvidenceText: "申请截止 5 月 20 日", OfficialURL: "https://jwc.whut.edu.cn/a.htm"},
	})
	if len(citations) != 1 || citations[0].Index != 1 || citations[0].SourceID != "good" {
		t.Fatalf("unexpected citations: %#v", citations)
	}
}

func TestBuildPrefersNoInventedCitationForMissingEvidence(t *testing.T) {
	if got := Build([]Candidate{{SourceID: "s", Title: "通知", OfficialURL: "https://www.whut.edu.cn"}}); len(got) != 0 {
		t.Fatalf("expected no citation, got %#v", got)
	}
}

func TestBuildRejectsPrivateNetworkURL(t *testing.T) {
	if got := Build([]Candidate{{SourceID: "s", Title: "通知", EvidenceText: "内容", OfficialURL: "http://127.0.0.1/private"}}); len(got) != 0 {
		t.Fatalf("private network URL must not be exposed: %#v", got)
	}
}
