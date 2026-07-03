package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCommonDir(t *testing.T) {
	// commonDir calls os.Stat on the result, so paths must exist on disk.
	// We use t.TempDir() to create a real but minimal directory tree.
	base := t.TempDir() // e.g. /tmp/TestCommonDir123

	// Create subdirectories we need.
	dirA := filepath.Join(base, "a")
	dirAB := filepath.Join(base, "a", "b")
	dirAC := filepath.Join(base, "a", "c")
	dirABC := filepath.Join(base, "a", "b", "c")
	dirABD := filepath.Join(base, "a", "b", "d")
	for _, d := range []string{dirA, dirAB, dirAC, dirABC, dirABD} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{
			name:  "single file",
			files: []string{filepath.Join(dirABC, "file.go")},
			want:  dirABC,
		},
		{
			name:  "two files same dir",
			files: []string{filepath.Join(dirAB, "file1.go"), filepath.Join(dirAB, "file2.go")},
			want:  dirAB,
		},
		{
			name:  "two files different dirs same parent",
			files: []string{filepath.Join(dirAB, "file.go"), filepath.Join(dirAC, "file.go")},
			want:  dirA,
		},
		{
			name:  "three files nested",
			files: []string{filepath.Join(dirABC, "f.go"), filepath.Join(dirABD, "f.go"), filepath.Join(dirAB, "f.go")},
			want:  dirAB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commonDir(tt.files)
			if got != tt.want {
				t.Errorf("commonDir(%v) = %q; want %q", tt.files, got, tt.want)
			}
		})
	}
}

func TestAbs64(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"positive", 3.14, 3.14},
		{"negative", -3.14, 3.14},
		{"zero", 0.0, 0.0},
		{"negative zero", math.Copysign(0, -1), 0.0},
		{"large positive", 1e15, 1e15},
		{"large negative", -1e15, 1e15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := abs64(tt.input)
			if got != tt.want {
				t.Errorf("abs64(%v) = %v; want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCommonDir_EmptyFiles(t *testing.T) {
	// Empty input should return the working directory - not panic.
	got := commonDir([]string{})
	if got == "" {
		t.Error("commonDir(empty) returned empty string; want a non-empty path")
	}
}

// TestGenerate_TokenBudget verifies that output length respects the token budget.
// A tight budget should produce less output than a generous one.
// We need cross-file references so rankTags produces ranked definition output
// to binary-search against - single-file inputs produce only filename entries.
func TestGenerate_TokenBudget(t *testing.T) {
	dir := t.TempDir()

	// funcs.go defines many functions that main.go references - this creates
	// cross-file edges so PageRank produces definition-level ranked output.
	writeFile(t, filepath.Join(dir, "funcs.go"), `package main

func Alpha() int   { return 1 }
func Bravo() int   { return 2 }
func Charlie() int { return 3 }
func Delta() int   { return 4 }
func Echo() int    { return 5 }
func Foxtrot() int { return 6 }
func Golf() int    { return 7 }
func Hotel() int   { return 8 }
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() {
	_ = Alpha()
	_ = Bravo()
	_ = Charlie()
	_ = Delta()
	_ = Echo()
	_ = Foxtrot()
	_ = Golf()
	_ = Hotel()
}
`)

	small, err := Generate([]string{dir}, nil, nil, nil, nil, "", 10, false)
	if err != nil {
		t.Fatalf("Generate(tokens=10) error = %v", err)
	}
	large, err := Generate([]string{dir}, nil, nil, nil, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate(tokens=2000) error = %v", err)
	}

	if len(small) >= len(large) {
		t.Errorf("expected small-budget output (%d chars) < large-budget output (%d chars)", len(small), len(large))
	}
}

// TestToTree_DiskFallback verifies that toTree reads source from disk via
// tag.Fname when the file is not present in srcStore.
func TestToTree_DiskFallback(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc FallbackFunc() {}\n"
	writeFile(t, fpath, src)

	tags := []Tag{
		{RelFname: "main.go", Fname: fpath, Line: 2, Name: "FallbackFunc", Kind: kindDef},
	}

	// Pass empty srcStore and treeStore - forces disk read inside toTree.
	got := toTree(tags, nil, "", nil, map[string]*sitter.Tree{}, map[string][]byte{})

	if !strings.Contains(got, "main.go") {
		t.Errorf("expected filename in disk-fallback output, got:\n%s", got)
	}
}
