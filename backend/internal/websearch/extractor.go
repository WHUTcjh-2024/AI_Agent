package websearch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

type HTMLExtractor struct {
	maxSections int
	maxChars    int
}

func NewHTMLExtractor(maxSections, maxChars int) *HTMLExtractor {
	if maxSections <= 0 {
		maxSections = 3
	}
	if maxChars <= 0 {
		maxChars = 1200
	}
	return &HTMLExtractor{maxSections: maxSections, maxChars: maxChars}
}

func (e *HTMLExtractor) ExtractRelevantSections(ctx context.Context, query string, page Page) ([]Section, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var blocks []string
	if page.ContentType == "text/plain" {
		blocks = splitPlainText(page.Body)
	} else {
		document, err := html.Parse(strings.NewReader(page.Body))
		if err != nil {
			return nil, fmt.Errorf("parse HTML page: %w", err)
		}
		blocks = visibleBlocks(document)
	}
	terms := queryTerms(query)
	sections := make([]Section, 0, len(blocks))
	for _, block := range blocks {
		text := normalizeSpace(block)
		if len([]rune(text)) < 8 {
			continue
		}
		score := relevanceScore(strings.ToLower(text), terms)
		sections = append(sections, Section{Text: text, Score: score})
	}
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].Score > sections[j].Score })
	if len(sections) > e.maxSections {
		sections = sections[:e.maxSections]
	}
	remaining := e.maxChars
	result := make([]Section, 0, len(sections))
	for _, section := range sections {
		if remaining <= 0 {
			break
		}
		runes := []rune(section.Text)
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		section.Text = string(runes)
		remaining -= len(runes)
		result = append(result, section)
	}
	return result, nil
}

func visibleBlocks(root *html.Node) []string {
	blocks := make([]string, 0, 32)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			switch tag {
			case "script", "style", "nav", "header", "footer", "form", "svg", "noscript":
				return
			case "p", "li", "h1", "h2", "h3", "td":
				if text := textContent(node); text != "" {
					blocks = append(blocks, text)
				}
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return blocks
}

func textContent(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return normalizeSpace(builder.String())
}

func splitPlainText(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == '\r' })
}

func normalizeSpace(value string) string { return strings.Join(strings.Fields(value), " ") }

func queryTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	terms := make([]string, 0, 12)
	seen := map[string]struct{}{}
	appendTerm := func(term string) {
		term = strings.TrimSpace(term)
		if len([]rune(term)) < 2 {
			return
		}
		if _, exists := seen[term]; exists {
			return
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	appendTerm(query)
	words := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		appendTerm(word)
		runes := []rune(word)
		if len(runes) > 4 {
			for index := 0; index+2 <= len(runes) && len(terms) < 12; index++ {
				appendTerm(string(runes[index : index+2]))
			}
		}
	}
	return terms
}

func relevanceScore(text string, terms []string) int {
	score := 0
	for index, term := range terms {
		count := strings.Count(text, term)
		if count == 0 {
			continue
		}
		weight := 1
		if index == 0 {
			weight = 8
		}
		score += count * weight
	}
	return score
}
