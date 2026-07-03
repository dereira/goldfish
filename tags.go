package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Tag is a single symbol extracted from a source file.
type Tag struct {
	RelFname string `json:"rel_fname"`
	Fname    string `json:"fname"`
	Line     int    `json:"line"` // 0-based
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

const (
	kindDef  = "def"
	kindRef  = "ref"
	kindFile = "file" // sentinel: file has no extractable tags, list filename only
)

// cacheEntry is what we persist to disk per file.
type cacheEntry struct {
	Mtime float64 `json:"mtime"`
	Tags  []Tag   `json:"tags"`
}

// tagCache is a simple mtime-keyed disk cache backed by a JSON file per cached
// source file. The cache directory lives at <root>/.goldfish.tags.cache/.
type tagCache struct {
	dir string
}

func newTagCache(dir string) *tagCache {
	return &tagCache{dir: dir}
}

func (c *tagCache) keyPath(fname string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(fname)
	return filepath.Join(c.dir, safe+".json")
}

func (c *tagCache) load(fname string, mtime float64) ([]Tag, bool) {
	data, err := os.ReadFile(c.keyPath(fname))
	if err != nil {
		return nil, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if entry.Mtime != mtime {
		return nil, false
	}
	return entry.Tags, true
}

func (c *tagCache) store(fname string, mtime float64, tags []Tag) {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(cacheEntry{Mtime: mtime, Tags: tags})
	if err != nil {
		return
	}
	_ = os.WriteFile(c.keyPath(fname), data, 0o600)
}

// getTags returns the tags for fname, using the cache when the file is
// unchanged. It also returns the parsed tree so callers can reuse it for
// context rendering without a second parse.
func getTags(fname, relFname string, cache *tagCache) ([]Tag, *sitter.Tree, []byte) {
	info, err := os.Stat(fname)
	if err != nil {
		return nil, nil, nil
	}
	mtime := float64(info.ModTime().UnixNano()) / 1e9

	if cached, ok := cache.load(fname, mtime); ok {
		return cached, nil, nil
	}

	tags, tree, src := extractTags(fname, relFname)
	cache.store(fname, mtime, tags)
	return tags, tree, src
}

// extractTags parses fname with tree-sitter and runs the language's .scm query
// to collect def/ref tags. Returns the tags, the live parse tree, and the
// source bytes (so the caller can reuse both for context rendering).
//
//nolint:funlen // single cohesive pipeline: parse → query → iterate captures → backfill refs
func extractTags(fname, relFname string) ([]Tag, *sitter.Tree, []byte) {
	entry, ok := langForFile(fname)
	if !ok {
		return nil, nil, nil
	}

	scm, err := loadSCM(entry)
	if err != nil {
		return nil, nil, nil
	}

	src, err := os.ReadFile(fname)
	if err != nil {
		return nil, nil, nil
	}

	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(entry.language); err != nil {
		return nil, nil, nil
	}

	tree := parser.Parse(src, nil)
	// tree is intentionally not closed here - ownership transfers to the caller.

	query, qErr := sitter.NewQuery(entry.language, scm)
	if qErr != nil {
		tree.Close()
		return nil, nil, nil
	}
	defer query.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	captures := cursor.Captures(query, tree.RootNode(), src)
	captureNames := query.CaptureNames()

	var tags []Tag
	sawRef := false
	sawDef := false

	seen := map[string]bool{}

	for match, captureIdx := captures.Next(); match != nil; match, captureIdx = captures.Next() {
		capture := match.Captures[captureIdx]
		captureName := captureNames[capture.Index]

		var kind string
		switch {
		case strings.HasPrefix(captureName, "name.definition."):
			kind = kindDef
			sawDef = true
		case strings.HasPrefix(captureName, "name.reference."):
			kind = kindRef
			sawRef = true
		default:
			continue
		}

		name := capture.Node.Utf8Text(src)
		line := int(capture.Node.StartPosition().Row)

		key := fmt.Sprintf("%s|%d|%s", kind, line, name)
		if seen[key] {
			continue
		}
		seen[key] = true

		tags = append(tags, Tag{
			RelFname: relFname,
			Fname:    fname,
			Line:     line,
			Name:     name,
			Kind:     kind,
		})
	}

	// Some language queries only emit definitions, not references (C and C++ are
	// common examples). Without any refs, those files would have no outgoing edges
	// in the symbol graph and PageRank would assign them zero weight, making them
	// invisible in the map. Backfilling a ref for each def name gives those files
	// self-referential edges so they remain rankable.
	if sawDef && !sawRef {
		defNames := map[string]bool{}
		for _, t := range tags {
			if t.Kind == "def" {
				defNames[t.Name] = true
			}
		}
		for name := range defNames {
			tags = append(tags, Tag{
				RelFname: relFname,
				Fname:    fname,
				Line:     -1,
				Name:     name,
				Kind:     kindRef,
			})
		}
	}

	return tags, tree, src
}
