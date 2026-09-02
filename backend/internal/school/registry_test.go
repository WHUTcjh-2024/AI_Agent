package school

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequiresKnowledgeVersionForCacheInvalidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "school.yaml")
	data := []byte("school_id: whut\nschool_name: Wuhan University of Technology\nallowed_domains:\n  - whut.edu.cn\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("school without knowledge_version must be rejected")
	}
}

func TestRegistryResolvesSchoolScopedKnowledgeVersion(t *testing.T) {
	registry := &Registry{current: Context{ID: "whut", KnowledgeVersion: "v3"}}
	version, err := registry.KnowledgeVersion("whut")
	if err != nil || version != "v3" {
		t.Fatalf("unexpected knowledge version: %q err=%v", version, err)
	}
	if _, err := registry.KnowledgeVersion("hzau"); err == nil {
		t.Fatal("unknown school must not inherit another school's version")
	}
}
