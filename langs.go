package main

import (
	"embed"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

//go:embed queries/*.scm
var queriesFS embed.FS

// Language name constants - used as keys in registry, extToLang, and scopeTypes.
const (
	langGo         = "go"
	langPython     = "python"
	langJavaScript = "javascript"
	langTypeScript = "typescript"
	langTSX        = "tsx"
	langRust       = "rust"
	langJava       = "java"
	langC          = "c"
	langCPP        = "cpp"
	langRuby       = "ruby"
	langCSharp     = "c_sharp"
)

// langEntry holds the tree-sitter language and the name of its .scm query file.
type langEntry struct {
	language *sitter.Language
	scmName  string
}

var registry = map[string]langEntry{
	langGo:         {sitter.NewLanguage(tree_sitter_go.Language()), langGo},
	langPython:     {sitter.NewLanguage(tree_sitter_python.Language()), langPython},
	langJavaScript: {sitter.NewLanguage(tree_sitter_javascript.Language()), langJavaScript},
	langTypeScript: {sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()), langTypeScript},
	langTSX:        {sitter.NewLanguage(tree_sitter_typescript.LanguageTSX()), langJavaScript},
	langRust:       {sitter.NewLanguage(tree_sitter_rust.Language()), langRust},
	langJava:       {sitter.NewLanguage(tree_sitter_java.Language()), langJava},
	langC:          {sitter.NewLanguage(tree_sitter_c.Language()), langC},
	langCPP:        {sitter.NewLanguage(tree_sitter_cpp.Language()), langCPP},
	langRuby:       {sitter.NewLanguage(tree_sitter_ruby.Language()), langRuby},
	langCSharp:     {sitter.NewLanguage(tree_sitter_c_sharp.Language()), "csharp"},
}

var extToLang = map[string]string{
	".go":   langGo,
	".py":   langPython,
	".js":   langJavaScript,
	".mjs":  langJavaScript,
	".cjs":  langJavaScript,
	".jsx":  langJavaScript,
	".ts":   langTypeScript,
	".tsx":  langTSX,
	".rs":   langRust,
	".java": langJava,
	".c":    langC,
	".h":    langC,
	".cc":   langCPP,
	".cpp":  langCPP,
	".cxx":  langCPP,
	".hpp":  langCPP,
	".hh":   langCPP,
	".rb":   langRuby,
	".cs":   langCSharp,
}

// langForFile returns the langEntry for a given file path, if supported.
func langForFile(fname string) (langEntry, bool) {
	ext := strings.ToLower(filepath.Ext(fname))
	lang, ok := extToLang[ext]
	if !ok {
		return langEntry{}, false
	}
	entry, ok := registry[lang]
	return entry, ok
}

// loadSCM reads the embedded query file for a language entry.
func loadSCM(entry langEntry) (string, error) {
	data, err := queriesFS.ReadFile("queries/" + entry.scmName + "-tags.scm")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
