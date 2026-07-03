package main

import (
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// parseForTest parses src using the language from the registry and returns the
// tree and source bytes. Parser and tree are closed automatically via t.Cleanup.
//
//nolint:unparam // The lang parameter is currently always langGo, but we keep it for future flexibility.
func parseForTest(t *testing.T, lang, src string) (tree *sitter.Tree, code []byte) {
	t.Helper()
	entry, ok := registry[lang]
	if !ok {
		t.Fatalf("parseForTest: unknown language %q", lang)
	}
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(entry.language); err != nil {
		t.Fatalf("parseForTest: SetLanguage: %v", err)
	}
	code = []byte(src)
	tree = parser.Parse(code, nil)
	t.Cleanup(tree.Close)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parseForTest: parse returned nil tree")
	}
	return tree, code
}

// --- renderFile ---

func TestRenderFile_EmptyInputs(t *testing.T) {
	tree, src := parseForTest(t, langGo, "package main\n\nfunc foo() {}\n")

	tests := []struct {
		name string
		src  []byte
		lois []int
		want string
	}{
		{"no lois", src, []int{}, ""},
		{"empty src", []byte{}, []int{0}, ""},
		{"nil src", nil, []int{0}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderFile("", tt.src, tree, tt.lois, langGo)
			if got != tt.want {
				t.Errorf("renderFile() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestRenderFile_SingleLine(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\treturn\n}\n"
	tree, code := parseForTest(t, langGo, src)

	// Line 3 (0-based) is "\treturn" - inside func foo.
	// renderFile should also pull in line 2 ("func foo() {") as the scope header.
	got := renderFile("", code, tree, []int{3}, langGo)

	if !strings.Contains(got, "func foo()") {
		t.Errorf("expected scope header 'func foo()' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "return") {
		t.Errorf("expected 'return' line in output, got:\n%s", got)
	}
}

func TestRenderFile_GapCollapsedToEllipsis(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\tx := 1\n\ty := 2\n\tz := 3\n}\n"
	tree, code := parseForTest(t, langGo, src)

	// Show only line 3 (x := 1) and line 5 (z := 3) - line 4 (y := 2) is a gap.
	got := renderFile("", code, tree, []int{3, 5}, langGo)

	if !strings.Contains(got, "⋮") {
		t.Errorf("expected gap marker '⋮' between non-adjacent lines, got:\n%s", got)
	}
}

func TestRenderFile_LongLineTruncated(t *testing.T) {
	longLine := strings.Repeat("x", 120)
	src := "package main\n\nvar _ = \"" + longLine + "\"\n"
	// No tree needed - tree=nil skips scope expansion, loi is line 2.
	got := renderFile("", []byte(src), nil, []int{2}, langGo)

	for line := range strings.SplitSeq(got, "\n") {
		if len(line) > 100 {
			t.Errorf("line exceeds 100 chars (%d): %q", len(line), line)
		}
	}
}

func TestRenderFile_OutOfBoundsLoisIgnored(t *testing.T) {
	src := "package main\n"
	got := renderFile("", []byte(src), nil, []int{-1, 999}, langGo)
	if got != "" {
		t.Errorf("out-of-bounds lois should produce empty output, got %q", got)
	}
}

func TestRenderFile_NilTree(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\treturn\n}\n"
	// With tree=nil, no scope header expansion - only the exact loi line shown.
	got := renderFile("", []byte(src), nil, []int{3}, langGo)
	if !strings.Contains(got, "return") {
		t.Errorf("expected 'return' in output, got %q", got)
	}
	// Without scope expansion, func declaration line should NOT appear.
	if strings.Contains(got, "func foo") {
		t.Errorf("did not expect scope header without a tree, got:\n%s", got)
	}
}

// --- addScopeHeaders ---

func TestAddScopeHeaders_GoFunction(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\treturn\n}\n"
	tree, _ := parseForTest(t, langGo, src)

	shown := map[int]bool{3: true} // line 3 = "\treturn"
	addScopeHeaders(tree, 3, langGo, shown, strings.Count(src, "\n")+1)

	// Line 2 is "func foo() {" - should be pulled in as scope header.
	if !shown[2] {
		t.Errorf("expected line 2 (func declaration) to be marked shown, got shown=%v", shown)
	}
}

func TestAddScopeHeaders_UnknownLanguage(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\treturn\n}\n"
	tree, _ := parseForTest(t, langGo, src)

	shown := map[int]bool{3: true}
	addScopeHeaders(tree, 3, "unknown_lang", shown, 10)

	// Unknown language - no extra lines should be added.
	if len(shown) != 1 {
		t.Errorf("unknown language should not expand shown set, got %v", shown)
	}
}

func TestAddScopeHeaders_NestedGoFunction(t *testing.T) {
	// Two functions: bar starts at line 6, its body at line 7.
	// addScopeHeaders on line 7 should pull in line 6 (func bar declaration).
	src := "package main\n\nfunc foo() {\n\treturn\n}\n\nfunc bar() {\n\treturn\n}\n"
	tree, _ := parseForTest(t, langGo, src)

	numLines := strings.Count(src, "\n") + 1
	shown := map[int]bool{7: true} // line 7 = "\treturn" inside bar
	addScopeHeaders(tree, 7, langGo, shown, numLines)

	// Line 6 is "func bar() {" - must be pulled in as scope header.
	if !shown[6] {
		t.Errorf("expected line 6 (func bar declaration) to be marked shown, got shown=%v", shown)
	}
}

// --- toTree ---

func TestToTree_EmptyTags(t *testing.T) {
	got := toTree(nil, nil, "", nil, nil, nil)
	if got != "" {
		t.Errorf("toTree(nil tags) = %q; want empty string", got)
	}
}

func TestToTree_SentinelFileTag(t *testing.T) {
	// A kindFile tag with no lois should just list the filename.
	tags := []Tag{
		{RelFname: "main.go", Fname: "main.go", Line: -1, Kind: kindFile},
	}
	got := toTree(tags, nil, "", nil, map[string]*sitter.Tree{}, map[string][]byte{})

	if !strings.Contains(got, "main.go") {
		t.Errorf("expected filename in output, got %q", got)
	}
}

func TestToTree_ChatFilesExcluded(t *testing.T) {
	tags := []Tag{
		{RelFname: "chat.go", Fname: "chat.go", Line: -1, Kind: kindFile},
		{RelFname: "other.go", Fname: "other.go", Line: -1, Kind: kindFile},
	}
	chatFiles := map[string]bool{"chat.go": true}

	got := toTree(tags, chatFiles, "", nil, map[string]*sitter.Tree{}, map[string][]byte{})

	if strings.Contains(got, "chat.go") {
		t.Errorf("chat file should be excluded from output, got:\n%s", got)
	}
	if !strings.Contains(got, "other.go") {
		t.Errorf("expected non-chat file in output, got:\n%s", got)
	}
}

func TestToTree_WithSourceAndTree(t *testing.T) {
	src := "package main\n\nfunc foo() {\n\treturn\n}\n"
	tree, code := parseForTest(t, langGo, src)

	tags := []Tag{
		{RelFname: "main.go", Fname: "main.go", Line: 3, Name: "foo", Kind: kindDef},
	}
	treeStore := map[string]*sitter.Tree{"main.go": tree}
	srcStore := map[string][]byte{"main.go": code}

	got := toTree(tags, nil, "", nil, treeStore, srcStore)

	if !strings.Contains(got, "main.go") {
		t.Errorf("expected filename in output, got:\n%s", got)
	}
	if !strings.Contains(got, "func foo") {
		t.Errorf("expected 'func foo' scope header in output, got:\n%s", got)
	}
}

func TestToTree_PreservesFileOrder(t *testing.T) {
	tags := []Tag{
		{RelFname: "a.go", Fname: "a.go", Line: -1, Kind: kindFile},
		{RelFname: "b.go", Fname: "b.go", Line: -1, Kind: kindFile},
		{RelFname: "c.go", Fname: "c.go", Line: -1, Kind: kindFile},
	}

	got := toTree(tags, nil, "", nil, map[string]*sitter.Tree{}, map[string][]byte{})

	posA := strings.Index(got, "a.go")
	posB := strings.Index(got, "b.go")
	posC := strings.Index(got, "c.go")

	if posA >= posB || posB >= posC {
		t.Errorf("expected file order a < b < c in output, got:\n%s", got)
	}
}
