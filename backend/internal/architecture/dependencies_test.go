package architecture

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestModuleDependencyBoundaries(t *testing.T) {
	_, currentFile, _, _ := runtime.Caller(0)
	internalRoot := filepath.Dir(filepath.Dir(currentFile))
	rules := map[string][]string{
		"domain":        {"asku/backend/internal/agent", "asku/backend/internal/api", "asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/run"},
		"run":           {"asku/backend/internal/llm", "asku/backend/internal/knowledge", "asku/backend/internal/websearch", "asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api"},
		"api":           {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/llm", "asku/backend/internal/knowledge", "asku/backend/internal/websearch"},
		"auth":          {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api"},
		"cache":         {"asku/backend/internal/agent", "asku/backend/internal/api", "asku/backend/internal/domain", "asku/backend/internal/knowledge", "asku/backend/internal/run", "asku/backend/internal/store", "asku/backend/internal/websearch"},
		"llm":           {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api", "asku/backend/internal/run", "asku/backend/internal/agent"},
		"knowledge":     {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api", "asku/backend/internal/run", "asku/backend/internal/agent"},
		"websearch":     {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api", "asku/backend/internal/run", "asku/backend/internal/agent"},
		"citation":      {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api", "asku/backend/internal/run", "asku/backend/internal/agent", "asku/backend/internal/knowledge", "asku/backend/internal/websearch"},
		"observability": {"asku/backend/internal/store", "asku/backend/internal/cache", "asku/backend/internal/api", "asku/backend/internal/run", "asku/backend/internal/agent", "asku/backend/internal/knowledge", "asku/backend/internal/websearch", "asku/backend/internal/domain"},
	}
	for packageName, forbidden := range rules {
		t.Run(packageName, func(t *testing.T) {
			assertNoImports(t, filepath.Join(internalRoot, packageName), forbidden)
		})
	}
}

func assertNoImports(t *testing.T, directory string, forbidden []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			for _, prefix := range forbidden {
				if path == prefix || strings.HasPrefix(path, prefix+"/") {
					t.Fatalf("%s imports forbidden dependency %s", filepath.Base(file), path)
				}
			}
		}
	}
}
