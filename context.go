package main

import (
	"os"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// AST node kind constants used across multiple language scope maps.
const (
	nodeFunctionDeclaration  = "function_declaration"
	nodeMethodDeclaration    = "method_declaration"
	nodeFunctionDefinition   = "function_definition"
	nodeClassDeclaration     = "class_declaration"
	nodeInterfaceDeclaration = "interface_declaration"
	// Shared across JS, TS, and TSX scope maps (goconst flags 3+ duplicate literals).
	nodeFunctionExpr  = "function"
	nodeArrowFunction = "arrow_function"
	nodeMethodDef     = "method_definition"
	nodeLexicalDecl   = "lexical_declaration"
)

const (
	// loiExpansionFactor estimates how many lines each line-of-interest expands to
	// (itself plus up to 3 enclosing scope headers) - used to pre-size the shown map.
	loiExpansionFactor = 4
	// maxLineLen caps rendered line width to guard against minified or generated code.
	maxLineLen = 100
	// sentinelColumn is passed as the end-column to DescendantForPointRange to ensure
	// the full line is covered regardless of actual line length.
	sentinelColumn = 1 << 16
)

// scopeTypes maps language names to the AST node kinds that represent a scope
// header - the signature line we always show even when the body isn't interesting.
var scopeTypes = map[string]map[string]bool{
	langGo: {
		nodeFunctionDeclaration: true,
		nodeMethodDeclaration:   true,
		"type_spec":             true,
	},
	langPython: {
		nodeFunctionDefinition: true,
		"class_definition":     true,
	},
	langJavaScript: {
		nodeFunctionDeclaration: true,
		nodeFunctionExpr:        true,
		nodeArrowFunction:       true,
		nodeMethodDef:           true,
		nodeClassDeclaration:    true,
		nodeLexicalDecl:         true,
	},
	langTypeScript: {
		nodeFunctionDeclaration:  true,
		nodeFunctionExpr:         true,
		nodeArrowFunction:        true,
		nodeMethodDef:            true,
		nodeClassDeclaration:     true,
		nodeInterfaceDeclaration: true,
		"type_alias_declaration": true,
		nodeLexicalDecl:          true,
	},
	// TSX shares TypeScript's grammar node kinds (interface/type_alias/etc.),
	// plus JSX support. langNameForFile resolves .tsx files to "tsx" (a
	// distinct registry entry from "typescript"), so it needs its own key here
	// even though the scope node kinds are identical.
	langTSX: {
		nodeFunctionDeclaration:  true,
		nodeFunctionExpr:         true,
		nodeArrowFunction:        true,
		nodeMethodDef:            true,
		nodeClassDeclaration:     true,
		nodeInterfaceDeclaration: true,
		"type_alias_declaration": true,
		nodeLexicalDecl:          true,
	},
	langRust: {
		"function_item": true,
		"impl_item":     true,
		"struct_item":   true,
		"enum_item":     true,
		"trait_item":    true,
	},
	langJava: {
		nodeMethodDeclaration:     true,
		nodeClassDeclaration:      true,
		nodeInterfaceDeclaration:  true,
		"constructor_declaration": true,
	},
	langC: {
		nodeFunctionDefinition: true,
		"struct_specifier":     true,
	},
	langCPP: {
		nodeFunctionDefinition: true,
		"class_specifier":      true,
		"struct_specifier":     true,
		"function_declarator":  true,
	},
	langRuby: {
		"method":           true,
		"singleton_method": true,
		"class":            true,
		"module":           true,
	},
	langCSharp: {
		nodeMethodDeclaration:     true,
		nodeClassDeclaration:      true,
		nodeInterfaceDeclaration:  true,
		"constructor_declaration": true,
		"property_declaration":    true,
	},
}

// renderFile produces a contextual snippet for a single file given a set of
// interesting line numbers (0-based). It reuses the already-parsed tree from
// tag extraction. If tree is nil it re-parses the file.
//
// Lines of interest and their enclosing scope headers are shown verbatim.
// Gaps between shown lines are collapsed to a single "⋮" line.
// Lines longer than 100 characters are truncated (guards against minified code).
func renderFile(_ string, src []byte, tree *sitter.Tree, lois []int, langName string) string {
	if len(lois) == 0 || len(src) == 0 {
		return ""
	}

	lines := strings.Split(string(src), "\n")
	numLines := len(lines)

	// ~4 lines per loi: the line itself plus up to 3 scope headers.
	shown := make(map[int]bool, len(lois)*loiExpansionFactor)
	for _, loi := range lois {
		if loi < 0 || loi >= numLines {
			continue
		}
		shown[loi] = true

		if tree != nil {
			addScopeHeaders(tree, loi, langName, shown, numLines)
		}
	}

	if len(shown) == 0 {
		return ""
	}

	var sb strings.Builder
	prevShown := false
	for i, line := range lines {
		if shown[i] {
			// Truncate very long lines - guards against minified JS / generated code.
			if len(line) > maxLineLen {
				line = line[:maxLineLen]
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
			prevShown = true
		} else {
			if prevShown {
				sb.WriteString("⋮\n")
			}
			prevShown = false
		}
	}

	return sb.String()
}

// addScopeHeaders walks from the node at `line` up the parse tree and marks
// the first line of every enclosing scope node as shown.
func addScopeHeaders(tree *sitter.Tree, line int, langName string, shown map[int]bool, numLines int) {
	scopes, ok := scopeTypes[langName]
	if !ok {
		return
	}

	// 1<<16 covers any column - sentinel for DescendantForPointRange.
	startPoint := sitter.NewPoint(uint(line), 0)
	endPoint := sitter.NewPoint(uint(line), sentinelColumn)
	node := tree.RootNode().DescendantForPointRange(startPoint, endPoint)
	if node == nil {
		return
	}

	for n := node; n != nil; n = n.Parent() {
		if scopes[n.Kind()] {
			headerLine := int(n.StartPosition().Row)
			if headerLine >= 0 && headerLine < numLines {
				shown[headerLine] = true
			}
		}
	}
}

// toTree renders a ranked tag list into the full map text. It groups tags by
// file, renders each file's contextual snippet, and assembles the output.
// chatRelFnames are excluded from output.
func toTree(
	rankedTags []Tag,
	chatRelFnames map[string]bool,
	_ string, // root - unused, kept for call-site clarity
	_ *tagCache, // cache - unused, kept for potential future incremental rendering
	treeStore map[string]*sitter.Tree,
	srcStore map[string][]byte,
) string {
	if len(rankedTags) == 0 {
		return ""
	}

	order, groups := groupTagsByFile(rankedTags, chatRelFnames)

	var sb strings.Builder
	for _, relFname := range order {
		g := groups[relFname]

		if g.isFile && len(g.lois) == 0 {
			sb.WriteByte('\n')
			sb.WriteString(relFname)
			sb.WriteByte('\n')
			continue
		}

		src := resolveFileSource(relFname, srcStore, rankedTags)
		tree := treeStore[relFname]
		langName := langNameForFile(relFname)
		snippet := renderFile(relFname, src, tree, g.lois, langName)

		sb.WriteByte('\n')
		sb.WriteString(relFname)
		if snippet != "" {
			sb.WriteString(":\n")
			sb.WriteString(snippet)
		} else {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

type fileGroup struct {
	relFname string
	lois     []int
	isFile   bool // sentinel: no tags, just list the filename
}

// groupTagsByFile partitions ranked tags into per-file groups, preserving rank order.
// Tags belonging to chatRelFnames are excluded.
func groupTagsByFile(rankedTags []Tag, chatRelFnames map[string]bool) (order []string, groups map[string]*fileGroup) {
	order = []string{}
	groups = map[string]*fileGroup{}
	for _, tag := range rankedTags {
		if chatRelFnames[tag.RelFname] {
			continue
		}
		if _, exists := groups[tag.RelFname]; !exists {
			order = append(order, tag.RelFname)
			groups[tag.RelFname] = &fileGroup{relFname: tag.RelFname}
		}
		if tag.Kind == kindFile {
			groups[tag.RelFname].isFile = true
		} else if tag.Line >= 0 {
			groups[tag.RelFname].lois = append(groups[tag.RelFname].lois, tag.Line)
		}
	}
	return order, groups
}

// resolveFileSource returns source bytes for relFname from the live store,
// falling back to disk via the absolute path recorded in a matching tag.
func resolveFileSource(relFname string, srcStore map[string][]byte, rankedTags []Tag) []byte {
	if src := srcStore[relFname]; src != nil {
		return src
	}
	for _, t := range rankedTags {
		if t.RelFname == relFname {
			src, _ := os.ReadFile(t.Fname)
			return src
		}
	}
	return nil
}

// langNameForFile returns the language name string (matching scopeTypes keys)
// for a given filename. Returns "" for unsupported files.
func langNameForFile(fname string) string {
	entry, ok := langForFile(fname)
	if !ok {
		return ""
	}
	for name, e := range registry {
		if e.language == entry.language {
			return name
		}
	}
	return ""
}
