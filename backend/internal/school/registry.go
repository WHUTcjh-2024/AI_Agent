package school

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Context struct {
	ID                       string            `yaml:"school_id" json:"schoolId"`
	Name                     string            `yaml:"school_name" json:"schoolName"`
	AllowedDomains           []string          `yaml:"allowed_domains" json:"allowedDomains"`
	OfficialKnowledgeBaseID  string            `yaml:"official_knowledge_base_id" json:"officialKnowledgeBaseId"`
	CommunityKnowledgeBaseID string            `yaml:"community_knowledge_base_id" json:"communityKnowledgeBaseId"`
	KnowledgeVersion         string            `yaml:"knowledge_version" json:"knowledgeVersion"`
	SourceTags               map[string]string `yaml:"source_tags" json:"sourceTags"`
}

type Registry struct {
	current Context
}

func Load(path string) (*Registry, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("ASKU_SCHOOL_CONFIG must select an active school config")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read school config: %w", err)
	}
	var context Context
	if err := yaml.Unmarshal(data, &context); err != nil {
		return nil, fmt.Errorf("parse school config: %w", err)
	}
	if context.ID == "" || context.Name == "" || len(context.AllowedDomains) == 0 || context.KnowledgeVersion == "" {
		return nil, fmt.Errorf("school config must define school_id, school_name, allowed_domains and knowledge_version")
	}
	return &Registry{current: context}, nil
}

func (r *Registry) Current() Context { return r.current }

func (r *Registry) Get(id string) (Context, bool) {
	if id == r.current.ID {
		return r.current, true
	}
	return Context{}, false
}

// AllowedDomains satisfies the Web Search Gateway scope boundary without
// coupling the school package to search-specific types.
func (r *Registry) AllowedDomains(id string) ([]string, error) {
	context, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("unknown school %q", id)
	}
	return append([]string(nil), context.AllowedDomains...), nil
}

func (r *Registry) OfficialKnowledgeBaseID(id string) (string, error) {
	context, ok := r.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown school %q", id)
	}
	return context.OfficialKnowledgeBaseID, nil
}

func (r *Registry) SchoolName(id string) (string, error) {
	context, ok := r.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown school %q", id)
	}
	return context.Name, nil
}

func (r *Registry) KnowledgeVersion(id string) (string, error) {
	context, ok := r.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown school %q", id)
	}
	return context.KnowledgeVersion, nil
}
