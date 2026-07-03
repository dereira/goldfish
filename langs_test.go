package main

import (
	"strings"
	"testing"
)

func TestLangForFile(t *testing.T) {
	tests := []struct {
		name      string
		fname     string
		wantLang  string
		wantFound bool
	}{
		// Supported extensions
		{"go file", "main.go", langGo, true},
		{"python file", "script.py", langPython, true},
		{"javascript file", "index.js", langJavaScript, true},
		{"mjs file", "module.mjs", langJavaScript, true},
		{"cjs file", "common.cjs", langJavaScript, true},
		{"jsx file", "component.jsx", langJavaScript, true},
		{"typescript file", "app.ts", langTypeScript, true},
		{"tsx file", "component.tsx", langTSX, true},
		{"rust file", "lib.rs", langRust, true},
		{"java file", "Main.java", langJava, true},
		{"c file", "main.c", langC, true},
		{"h file", "header.h", langC, true},
		{"cc file", "impl.cc", langCPP, true},
		{"cpp file", "impl.cpp", langCPP, true},
		{"cxx file", "impl.cxx", langCPP, true},
		{"hpp file", "header.hpp", langCPP, true},
		{"hh file", "header.hh", langCPP, true},
		{"ruby file", "script.rb", langRuby, true},
		{"csharp file", "Program.cs", langCSharp, true},

		// Extension case-insensitivity
		{"uppercase .GO", "main.GO", langGo, true},
		{"mixed case .Py", "script.Py", langPython, true},

		// Unsupported / unknown
		{"text file", "notes.txt", "", false},
		{"markdown file", "README.md", "", false},
		{"yaml file", "config.yml", "", false},
		{"no extension", "Makefile", "", false},
		{"empty string", "", "", false},

		// Path with directories
		{"go file in subdir", "pkg/util/helper.go", langGo, true},
		{"ts file nested", "src/components/Button.ts", langTypeScript, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, found := langForFile(tt.fname)
			if found != tt.wantFound {
				t.Fatalf("langForFile(%q) found = %v; want %v", tt.fname, found, tt.wantFound)
			}
			if !tt.wantFound {
				return
			}
			// Verify the returned entry matches the expected language via the registry.
			wantEntry, ok := registry[tt.wantLang]
			if !ok {
				t.Fatalf("test bug: wantLang %q not in registry", tt.wantLang)
			}
			if entry.language != wantEntry.language {
				t.Errorf("langForFile(%q) language mismatch: got entry for %q family, want %q", tt.fname, entry.scmName, tt.wantLang)
			}
		})
	}
}

func TestLoadSCM(t *testing.T) {
	tests := []struct {
		name        string
		lang        string
		wantErr     bool
		wantContain string // a token expected in every well-formed tags .scm file
	}{
		{"go", langGo, false, "name.definition"},
		{"python", langPython, false, "name.definition"},
		{"javascript", langJavaScript, false, "name.definition"},
		{"typescript", langTypeScript, false, "name.definition"},
		{"rust", langRust, false, "name.definition"},
		{"java", langJava, false, "name.definition"},
		{"c", langC, false, "name.definition"},
		{"cpp", langCPP, false, "name.definition"},
		{"ruby", langRuby, false, "name.definition"},
		{"csharp", langCSharp, false, "name.definition"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := registry[tt.lang]
			if !ok {
				t.Fatalf("language %q not in registry", tt.lang)
			}
			scm, err := loadSCM(entry)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadSCM(%q) error = %v; wantErr %v", tt.lang, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if scm == "" {
				t.Errorf("loadSCM(%q) returned empty string", tt.lang)
			}
			if tt.wantContain != "" && !strings.Contains(scm, tt.wantContain) {
				t.Errorf("loadSCM(%q) result does not contain %q", tt.lang, tt.wantContain)
			}
		})
	}
}
