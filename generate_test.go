package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerate_AllGrammars is an integration test that exercises the full
// Generate pipeline (file collection → tag extraction → PageRank → rendering)
// for every supported language grammar.
//
// Each case writes a minimal but valid source file to a temp dir, calls
// Generate, and asserts that:
//  1. No error is returned.
//  2. The output is non-empty.
//  3. A known symbol name from the snippet appears in the output.
func TestGenerate_AllGrammars(t *testing.T) {
	tests := []struct {
		lang       string
		filename   string
		src        string
		wantSymbol string // expected in Generate output
	}{
		{
			lang:     "go",
			filename: "main.go",
			src: `package main

func HelloWorld() {
	return
}
`,
			wantSymbol: "HelloWorld",
		},
		{
			lang:     "python",
			filename: "main.py",
			src: `def hello_world():
    return 1

class MyClass:
    pass
`,
			wantSymbol: "hello_world",
		},
		{
			lang:     "javascript",
			filename: "main.js",
			src: `function helloWorld() {
  return 1;
}

const myArrow = () => 2;
`,
			wantSymbol: "helloWorld",
		},
		{
			lang:     "typescript",
			filename: "main.ts",
			src: `function helloWorld(): number {
  return 1;
}

interface MyInterface {
  name: string;
}
`,
			wantSymbol: "helloWorld",
		},
		{
			lang:     "tsx",
			filename: "main.tsx",
			src: `function HelloWorld(): JSX.Element {
  return null;
}
`,
			wantSymbol: "HelloWorld",
		},
		{
			lang:     "rust",
			filename: "main.rs",
			src: `fn hello_world() -> i32 {
    42
}

struct MyStruct {
    value: i32,
}
`,
			wantSymbol: "hello_world",
		},
		{
			lang:     "java",
			filename: "Main.java",
			src: `public class Main {
    public static void helloWorld() {
        return;
    }
}
`,
			wantSymbol: "helloWorld",
		},
		{
			lang:     "c",
			filename: "main.c",
			src: `int hello_world() {
    return 42;
}
`,
			wantSymbol: "hello_world",
		},
		{
			lang:     "cpp",
			filename: "main.cpp",
			src: `int hello_world() {
    return 42;
}

class MyClass {
public:
    int value;
};
`,
			wantSymbol: "hello_world",
		},
		{
			lang:     "ruby",
			filename: "main.rb",
			src: `def hello_world
  42
end

class MyClass
  def initialize
  end
end
`,
			wantSymbol: "hello_world",
		},
		{
			lang:     "csharp",
			filename: "Main.cs",
			src: `public class Program {
    public static void HelloWorld() {
        return;
    }
}
`,
			wantSymbol: "HelloWorld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			dir := t.TempDir()
			fpath := filepath.Join(dir, tt.filename)
			if err := os.WriteFile(fpath, []byte(tt.src), 0o600); err != nil {
				t.Fatalf("setup: write file: %v", err)
			}

			got, err := Generate([]string{fpath}, nil, nil, nil, nil, "", 2000, false)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if got == "" {
				t.Fatal("Generate() returned empty output")
			}
			if !strings.Contains(got, tt.wantSymbol) {
				t.Errorf("Generate() output does not contain symbol %q\noutput:\n%s", tt.wantSymbol, got)
			}
		})
	}
}

// TestGenerate_MentionedIdent verifies that passing a symbol via mentionedIdents
// causes it to appear in the output - the ranking bias is observable.
func TestGenerate_MentionedIdent(t *testing.T) {
	dir := t.TempDir()

	// targetFunc is defined in a.go and called from b.go - cross-file reference
	// flows TO a.go, so PageRank surfaces targetFunc as a definition.
	writeFile(t, filepath.Join(dir, "a.go"), `package main

func targetFunc() int {
	return 1
}
`)
	writeFile(t, filepath.Join(dir, "b.go"), `package main

func otherFunc() int {
	return targetFunc()
}
`)

	got, err := Generate([]string{dir}, nil, nil, []string{"targetFunc"}, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(got, "targetFunc") {
		t.Errorf("expected 'targetFunc' in output when hinted via mentionedIdents, got:\n%s", got)
	}
}

// TestGenerate_MentionedFname verifies that passing a filename via mentionedFnames
// causes its symbols to appear in the output.
func TestGenerate_MentionedFname(t *testing.T) {
	dir := t.TempDir()

	hinted := filepath.Join(dir, "important.go")
	// importantFunc is defined in important.go and called from other.go -
	// this creates a cross-file reference edge pointing TO important.go,
	// giving PageRank signal to surface importantFunc as a definition.
	writeFile(t, hinted, `package main

func importantFunc() int {
	return 42
}
`)
	writeFile(t, filepath.Join(dir, "other.go"), `package main

func otherFunc() int {
	return importantFunc()
}
`)

	got, err := Generate([]string{dir}, nil, []string{hinted}, nil, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(got, "importantFunc") {
		t.Errorf("expected 'importantFunc' in output when its file is hinted via mentionedFnames, got:\n%s", got)
	}
}

// TestGenerate_UnsupportedFile verifies that Generate returns an error when
// given a file with no supported language grammar.
func TestGenerate_UnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(fpath, []byte("hello world\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := Generate([]string{fpath}, nil, nil, nil, nil, "", 2000, false)
	if err == nil {
		t.Fatal("expected error for unsupported file, got nil")
	}
}

// TestGenerate_NonExistentPath verifies that Generate returns an error for a
// path that does not exist on disk.
func TestGenerate_NonExistentPath(t *testing.T) {
	_, err := Generate([]string{"/nonexistent/path/file.go"}, nil, nil, nil, nil, "", 2000, false)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

// writeFile is a test helper that writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}

// TestGenerate_ExcludePattern verifies that files matching an excludePattern are
// absent from the output while non-matching files remain.
func TestGenerate_ExcludePattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func Hello() {}
`)
	writeFile(t, filepath.Join(dir, "main_test.go"), `package main

import "testing"

func TestHello(t *testing.T) {}
`)

	got, err := Generate([]string{dir}, nil, nil, nil, []string{"*_test.go"}, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if strings.Contains(got, "main_test.go") {
		t.Errorf("expected main_test.go to be excluded, but it appeared in output:\n%s", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected main.go to appear in output, but it was missing:\n%s", got)
	}
}

// TestGenerate_NoDefaultExcludes verifies that passing excludePatterns=nil
// (no-default-excludes mode) includes test files in the output.
func TestGenerate_NoDefaultExcludes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main_test.go"), `package main

import "testing"

func TestSomething(t *testing.T) {}
`)

	got, err := Generate([]string{dir}, nil, nil, nil, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got == "" {
		t.Fatal("Generate() returned empty output; expected test file to be included")
	}
}

// TestGenerate_DefaultExcludesApplied verifies that applying defaultExcludes
// removes test files but keeps non-test files.
func TestGenerate_DefaultExcludesApplied(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func Hello() {}
`)
	writeFile(t, filepath.Join(dir, "main_test.go"), `package main

import "testing"

func TestHello(t *testing.T) {}
`)

	got, err := Generate([]string{dir}, nil, nil, nil, defaultExcludes, "", 2000, false)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if strings.Contains(got, "main_test.go") {
		t.Errorf("expected main_test.go to be excluded by defaultExcludes, but it appeared:\n%s", got)
	}
}
