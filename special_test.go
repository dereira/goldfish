package main

import (
	"reflect"
	"testing"
)

func TestIsImportant(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		want    bool
	}{
		// Known important filenames
		{"go.mod", "go.mod", true},
		{"go.sum", "go.sum", true},
		{"README.md", "README.md", true},
		{"README at root", "README", true},
		{"package.json", "package.json", true},
		{"Makefile", "Makefile", true},
		{"Dockerfile", "Dockerfile", true},
		{"docker-compose.yml", "docker-compose.yml", true},
		{"Cargo.toml", "Cargo.toml", true},
		{"pyproject.toml", "pyproject.toml", true},
		{"requirements.txt", "requirements.txt", true},
		{"tsconfig.json", "tsconfig.json", true},
		{"LICENSE", "LICENSE", true},
		{"LICENSE.md", "LICENSE.md", true},
		{"CHANGELOG.md", "CHANGELOG.md", true},
		{"SECURITY.md", "SECURITY.md", true},
		{".gitignore", ".gitignore", true},

		// GitHub Actions workflow files
		{"github workflow yml", ".github/workflows/ci.yml", true},
		{"github workflow yaml nested", ".github/workflows/release.yml", true},

		// Nested path - only the filename matters
		{"go.mod in subdir", "subpkg/go.mod", true},
		{"README.md in subdir", "docs/README.md", true},

		// Not important
		{"regular go file", "main.go", false},
		{"random yml", "config.yml", false},
		{"random json", "config.json", false},
		{"unknown extension", "notes.txt", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImportant(tt.relPath)
			if got != tt.want {
				t.Errorf("isImportant(%q) = %v; want %v", tt.relPath, got, tt.want)
			}
		})
	}
}

func TestFilterImportant(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "empty input",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "no important files",
			input: []string{"main.go", "handler.go", "config.yml"},
			want:  []string{},
		},
		{
			name:  "all important",
			input: []string{"go.mod", "README.md", "LICENSE"},
			want:  []string{"go.mod", "README.md", "LICENSE"},
		},
		{
			name:  "mixed",
			input: []string{"main.go", "go.mod", "handler.go", "README.md"},
			want:  []string{"go.mod", "README.md"},
		},
		{
			name:  "preserves order",
			input: []string{"README.md", "main.go", "go.mod", "util.go"},
			want:  []string{"README.md", "go.mod"},
		},
		{
			name:  "github workflow included",
			input: []string{"main.go", ".github/workflows/ci.yml", "go.mod"},
			want:  []string{".github/workflows/ci.yml", "go.mod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterImportant(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterImportant(%v)\n got  %v\n want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		{
			name:     "test go file matches *_test.go",
			relPath:  "foo_test.go",
			patterns: defaultExcludes,
			want:     true,
		},
		{
			name:     "vendor file matches vendor/**",
			relPath:  "vendor/some/pkg/file.go",
			patterns: defaultExcludes,
			want:     true,
		},
		{
			name:     "proto generated file matches *.pb.go",
			relPath:  "api/service.pb.go",
			patterns: defaultExcludes,
			want:     true,
		},
		{
			name:     "normal main.go does not match",
			relPath:  "main.go",
			patterns: defaultExcludes,
			want:     false,
		},
		{
			name:     "empty pattern list never matches",
			relPath:  "foo_test.go",
			patterns: []string{},
			want:     false,
		},
		{
			name:     "testdata fixtures match testdata/**",
			relPath:  "testdata/fixtures/data.json",
			patterns: defaultExcludes,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldExclude(tt.relPath, tt.patterns)
			if got != tt.want {
				t.Errorf("shouldExclude(%q, patterns) = %v; want %v", tt.relPath, got, tt.want)
			}
		})
	}
}
