package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultTokenBudget = 4096
	outputFileMode     = 0o600 // owner read/write only
)

// version is stamped at build time via:
//   -ldflags "-X main.version=v1.2.3"
// The Taskfile's build/build:release tasks set this from `git describe`;
// CI sets it from the release-please tag. Left as "dev" for local `go build`
// / `go run` with no ldflags.
var version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "Print version and exit.")
	chatFlag := flag.String("chat", "", "Comma-separated files to treat as 'in focus'. Biases ranking; excluded from output.")
	tokensFlag := flag.Int("tokens", defaultTokenBudget, "Target token budget for the map.")
	outputFlag := flag.String("output", "", "Write map to FILE instead of stdout.")
	verboseFlag := flag.Bool("verbose", false, "Print progress to stderr.")
	cacheDirFlag := flag.String("cache-dir", "", "Override cache directory (default: OS cache dir).")

	var mentionFileFlags stringSliceFlag
	var mentionIdentFlags stringSliceFlag
	var excludeFlags stringSliceFlag
	flag.Var(&mentionFileFlags, "mention-file", "File to boost in ranking (repeatable).")
	flag.Var(&mentionIdentFlags, "mention-ident", "Symbol name to boost in ranking (repeatable).")
	flag.Var(&excludeFlags, "exclude", "Glob pattern to exclude (matched against relative file path, repeatable).")
	noDefaultExcludesFlag := flag.Bool("no-default-excludes", false, "Disable built-in default exclusions.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: goldfish [flags] PATH [PATH...]\n\n")
		fmt.Fprintf(os.Stderr, "Generate a ranked, token-budgeted map of a code repository.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}

	paths := flag.Args()
	if len(paths) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	var chatFiles []string
	for f := range strings.SplitSeq(*chatFlag, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			abs, err := filepath.Abs(f)
			if err == nil {
				chatFiles = append(chatFiles, abs)
			}
		}
	}

	var mentionedFnames []string
	for _, f := range mentionFileFlags {
		abs, err := filepath.Abs(f)
		if err == nil {
			mentionedFnames = append(mentionedFnames, abs)
		}
	}

	excludePatterns := defaultExcludes
	if *noDefaultExcludesFlag {
		excludePatterns = nil
	}
	excludePatterns = append(excludePatterns, excludeFlags...)

	result, err := Generate(paths, chatFiles, mentionedFnames, mentionIdentFlags, excludePatterns, *cacheDirFlag, *tokensFlag, *verboseFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *outputFlag != "" {
		if err := os.WriteFile(*outputFlag, []byte(result), outputFileMode); err != nil {
			fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
			os.Exit(1)
		}
		if *verboseFlag {
			fmt.Fprintf(os.Stderr, "Map written to %s\n", *outputFlag)
		}
	} else {
		fmt.Print(result)
	}
}

// stringSliceFlag implements flag.Value for repeatable string flags.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ", ") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
