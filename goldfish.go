package main

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/git-pkgs/gitignore"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	charsPerToken = 4
	// charsPerTag is an empirical estimate used to seed the binary search starting point.
	charsPerTag = 25
)

// Generate builds a ranked, token-budgeted map of the given paths.
// chatFiles are excluded from output but bias ranking toward their callers/callees.
// mentionedFnames and mentionedIdents receive a ranking boost.
// excludePatterns are matched against relative file paths; pass nil to disable defaults.
// Returns the map as a string, or an error if no supported files were found.
func Generate(paths, chatFiles, mentionedFnames, mentionedIdents, excludePatterns []string,
	cacheDir string, tokens int, verbose bool,
) (string, error) {
	otherFiles, err := collectFiles(paths, excludePatterns)
	if err != nil {
		return "", err
	}
	if len(otherFiles) == 0 {
		return "", fmt.Errorf("no supported source files found")
	}

	allFiles := make([]string, len(otherFiles)+len(chatFiles))
	copy(allFiles, otherFiles)
	copy(allFiles[len(otherFiles):], chatFiles)
	root := commonDir(allFiles)

	if verbose {
		fmt.Fprintf(os.Stderr, "root: %s\n", root)
	}

	cache := newTagCache(resolveCacheDir(cacheDir, root))

	chatRelFnames := make(map[string]bool, len(chatFiles))
	for _, f := range chatFiles {
		rel, _ := filepath.Rel(root, f)
		chatRelFnames[rel] = true
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Scanning %d files...\n", len(otherFiles))
	}

	var allTags []Tag
	treeStore := map[string]*sitter.Tree{}
	srcStore := map[string][]byte{}

	for _, fname := range otherFiles {
		rel, _ := filepath.Rel(root, fname)
		tags, tree, src := getTags(fname, rel, cache)
		allTags = append(allTags, tags...)
		if tree != nil {
			treeStore[rel] = tree
			srcStore[rel] = src
		}
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Extracted %d tags from %d files\n", len(allTags), len(otherFiles))
	}

	mentionedFnamesSet := make(map[string]bool, len(mentionedFnames))
	for _, f := range mentionedFnames {
		rel, _ := filepath.Rel(root, f)
		mentionedFnamesSet[rel] = true
	}

	mentionedIdentsSet := make(map[string]bool, len(mentionedIdents))
	for _, id := range mentionedIdents {
		mentionedIdentsSet[id] = true
	}

	rankedTags := rankTags(allTags, chatRelFnames, mentionedFnamesSet, mentionedIdentsSet)
	rankedTags = prependImportant(rankedTags, otherFiles, root)

	result := binarySearchTree(rankedTags, chatRelFnames, root, cache, treeStore, srcStore, tokens, verbose)
	if result == "" {
		return "", fmt.Errorf("no map generated - token budget too small (try increasing --tokens)")
	}

	return result, nil
}

// collectFiles walks each path, respecting .gitignore, and returns a
// deduplicated list of absolute file paths. Files whose relative path matches
// any of excludePatterns are skipped.
func collectFiles(paths, excludePatterns []string) ([]string, error) {
	var files []string
	seen := map[string]bool{}

	add := func(abs string) {
		if !seen[abs] {
			seen[abs] = true
			files = append(files, abs)
		}
	}

	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve path %s: %w", p, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if !info.IsDir() {
			if !shouldExclude(filepath.Base(abs), excludePatterns) {
				add(abs)
			}
			continue
		}
		if err := gitignore.Walk(abs, func(path string, d fs.DirEntry) error {
			if !d.IsDir() {
				// gitignore.Walk returns paths relative to abs using OS separators;
				// normalise to forward slashes for shouldExclude (which uses path.Match).
				rel := filepath.ToSlash(path)
				if !shouldExclude(rel, excludePatterns) {
					add(filepath.Join(abs, path))
				}
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walk %s: %w", abs, err)
		}
	}

	return files, nil
}

// prependImportant prepends sentinel tags for important files (go.mod, README,
// etc.) that aren't already present in the ranked tag list.
func prependImportant(rankedTags []Tag, otherFiles []string, root string) []Tag {
	otherRelFnames := make([]string, 0, len(otherFiles))
	for _, f := range otherFiles {
		rel, _ := filepath.Rel(root, f)
		otherRelFnames = append(otherRelFnames, rel)
	}
	sort.Strings(otherRelFnames)

	rankedFnames := make(map[string]bool, len(rankedTags))
	for _, t := range rankedTags {
		rankedFnames[t.RelFname] = true
	}

	var special []Tag
	for _, rel := range filterImportant(otherRelFnames) {
		if !rankedFnames[rel] {
			special = append(special, Tag{RelFname: rel, Kind: kindFile})
		}
	}

	return append(special, rankedTags...)
}

// binarySearchTree finds the largest prefix of rankedTags that fits within the
// token budget and renders it to a string.
func binarySearchTree(
	rankedTags []Tag,
	chatRelFnames map[string]bool,
	root string,
	cache *tagCache,
	treeStore map[string]*sitter.Tree,
	srcStore map[string][]byte,
	tokens int,
	verbose bool,
) string {
	numTags := len(rankedTags)
	lo, hi := 0, numTags
	best := ""
	mid := min(tokens/charsPerTag, numTags)

	for lo <= hi {
		if verbose {
			fmt.Fprintf(os.Stderr, "Trying %d tags...\n", mid)
		}

		candidate := toTree(rankedTags[:mid], chatRelFnames, root, cache, treeStore, srcStore)
		count := len(candidate) / charsPerToken

		// 15% tolerance: exact token counts require a real tokenizer; a character
		// estimate close enough to the budget is indistinguishable in practice.
		okErr := 0.15
		pctErr := abs64(float64(count-tokens)) / float64(tokens)

		if (count <= tokens && len(candidate) > len(best)) || pctErr < okErr {
			best = candidate
			if pctErr < okErr {
				break
			}
		}

		if count < tokens {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
		mid = (lo + hi) / 2
	}

	return best
}

// commonDir returns the deepest common directory containing all files.
func commonDir(files []string) string {
	if len(files) == 0 {
		cwd, _ := os.Getwd()
		return cwd
	}
	root := filepath.Dir(files[0])
	for _, f := range files[1:] {
		d := filepath.Dir(f)
		for !strings.HasPrefix(d, root) {
			root = filepath.Dir(root)
			if root == "/" || root == "." {
				break
			}
		}
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		root = filepath.Dir(root)
	}
	return root
}

// resolveCacheDir returns the directory to use for the tag cache.
// If base is non-empty it is used as-is (user override via --cache-dir).
// Otherwise the OS default cache directory is used: on macOS ~/Library/Caches,
// on Linux $XDG_CACHE_HOME or ~/.cache, on Windows %LOCALAPPDATA%.
// The final path is <base>/goldfish/<sha256(root)[:12]>/.
func resolveCacheDir(base, root string) string {
	if base == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			// Fallback: use a temp dir - cache misses are fine, just slower.
			userCache = os.TempDir()
		}
		base = userCache
	}
	return filepath.Join(base, "goldfish", rootHash(root))
}

// rootHash returns a 12-character hex prefix of the SHA-256 of root.
func rootHash(root string) string {
	sum := sha256.Sum256([]byte(root))
	return fmt.Sprintf("%x", sum[:6])
}

func abs64(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
