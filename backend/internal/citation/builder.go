package citation

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"asku/backend/internal/domain"
)

type Candidate struct {
	SourceID           string
	AskUDocumentID     string
	WeKnoraKnowledgeID string
	ChunkID            string
	Title              string
	SourceName         string
	Department         string
	PublishDate        time.Time
	SourceType         string
	DocumentType       string
	OfficialURL        string
	AttachmentURL      string
	ParentPageURL      string
	EvidenceText       string
	Authority          string
	Freshness          string
	KnowledgeBundleID  string
}

// Build assigns citation indices only after validating that every candidate
// has evidence and a public HTTP(S) origin. Invalid/unrelated placeholders are
// omitted instead of being presented as answer evidence.
func Build(candidates []Candidate) []domain.Citation {
	result := make([]domain.Citation, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate.SourceID = strings.TrimSpace(candidate.SourceID)
		candidate.Title = strings.TrimSpace(candidate.Title)
		candidate.EvidenceText = truncate(strings.TrimSpace(candidate.EvidenceText), 1200)
		candidate.OfficialURL = publicURL(candidate.OfficialURL)
		candidate.AttachmentURL = publicURL(candidate.AttachmentURL)
		candidate.ParentPageURL = publicURL(candidate.ParentPageURL)
		if candidate.SourceID == "" || candidate.Title == "" || candidate.EvidenceText == "" ||
			first(candidate.AttachmentURL, candidate.OfficialURL, candidate.ParentPageURL) == "" {
			continue
		}
		identity := strings.Join([]string{candidate.SourceID, candidate.AskUDocumentID, candidate.WeKnoraKnowledgeID, candidate.ChunkID}, "\x00")
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		digest := sha256.Sum256([]byte(identity))
		sourceName := first(strings.TrimSpace(candidate.SourceName), strings.TrimSpace(candidate.Department), candidate.Title)
		department := first(strings.TrimSpace(candidate.Department), sourceName)
		result = append(result, domain.Citation{
			CitationID: fmt.Sprintf("cit_%x", digest[:12]), Index: len(result) + 1,
			SourceID: candidate.SourceID, AskUDocumentID: strings.TrimSpace(candidate.AskUDocumentID),
			WeKnoraKnowledgeID: strings.TrimSpace(candidate.WeKnoraKnowledgeID), ChunkID: strings.TrimSpace(candidate.ChunkID),
			Title: candidate.Title, SourceName: sourceName, Department: department,
			PublishDate: candidate.PublishDate.UTC(), SourceType: strings.TrimSpace(candidate.SourceType),
			DocumentType: strings.TrimSpace(candidate.DocumentType), OfficialURL: candidate.OfficialURL,
			AttachmentURL: candidate.AttachmentURL, ParentPageURL: candidate.ParentPageURL,
			EvidenceText: candidate.EvidenceText, Authority: first(strings.TrimSpace(candidate.Authority), "OFFICIAL_DEPARTMENT"),
			Freshness: strings.TrimSpace(candidate.Freshness), KnowledgeBundleID: strings.TrimSpace(candidate.KnowledgeBundleID),
		})
	}
	return result
}

func publicURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "localhost" || strings.HasSuffix(hostname, ".local") {
		return ""
	}
	if address := net.ParseIP(hostname); address != nil && (address.IsPrivate() || address.IsLoopback() || address.IsUnspecified() || address.IsLinkLocalUnicast()) {
		return ""
	}
	return parsed.String()
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
