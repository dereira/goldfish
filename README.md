# gogoldfish

Generate a ranked, token-budgeted map of a code repository.

## Why

When you give an LLM a codebase, you can't paste every file - context windows have limits. Dumping raw files wastes tokens on boilerplate and buries the important parts. Goldfish solves this by figuring out which parts of the code matter most and showing just enough structure for the LLM to understand the codebase.

It works by building a symbol graph across files - every definition and reference - then running PageRank to surface what's most connected. The output is a structured sketch: function signatures, type declarations, class definitions, with `⋮` where bodies are omitted. Enough to reason about the code, not enough to blow your budget.

## Credits

The repo-map algorithm - tag extraction via tree-sitter, PageRank over a symbol graph, `--chat`-file biasing, token-budgeted rendering - is a Go port of the approach pioneered by [aider](https://github.com/Aider-AI/aider)'s repository map, described in ["Building a better repository map with tree sitter"](https://aider.chat/2023/10/22/repomap.html). All credit for the core algorithm design goes to the aider project.

The `.scm` tag-query files under [`queries/`](./queries) are adapted from aider's own vendored queries (`aider/queries/tree-sitter-language-pack/`), which are themselves modified versions of the `tags.scm` files shipped by each language's tree-sitter grammar (e.g. [tree-sitter-go](https://github.com/tree-sitter/tree-sitter-go), MIT licensed). aider is Apache-2.0 licensed.

## Install

```bash
go install github.com/dereira/goldfish@latest
```

Or build from source:

```bash
git clone https://github.com/dereira/goldfish
cd goldfish
go build -o goldfish .
```

## Usage

```bash
# Map an entire directory
goldfish /path/to/repo

# Focus ranking on specific files (comma-separated, no spaces)
# Their callers and callees rank higher. Excluded from output -
# assumed to already be in your LLM context.
goldfish --chat src/auth.go,src/models.go /path/to/repo

# Boost specific files or symbols relevant to your current task
# --mention-file and --mention-ident are repeatable
goldfish --mention-file src/auth.go --mention-ident handleLogin /path/to/repo

# Set a token budget (default: 4096)
goldfish --tokens 8192 /path/to/repo

# Write to a file instead of stdout
goldfish --output map.txt /path/to/repo

# Show progress
goldfish --verbose /path/to/repo

# Map multiple directories or individual files
goldfish src/ lib/
goldfish main.go src/ internal/

# Add extra exclusions on top of defaults
goldfish --exclude "fixtures/**" --exclude "mocks/**" /path/to/repo

# Disable all default exclusions (include test files, vendor, etc.)
goldfish --no-default-excludes /path/to/repo
```

## Flags

| Flag                    | Default | Description                                                                                                      |
| ----------------------- | ------- | ---------------------------------------------------------------------------------------------------------------- |
| `--chat`                | -       | Comma-separated files to treat as "in focus". Their callers and callees rank higher. Excluded from output.       |
| `--mention-file`        | -       | Boost a specific file in ranking. Repeatable.                                                                    |
| `--mention-ident`       | -       | Boost a specific symbol name in ranking. Repeatable.                                                             |
| `--exclude`             | -       | Glob pattern to exclude from the map (matched against relative path). Repeatable. Adds to defaults.              |
| `--no-default-excludes` | `false` | Disable built-in default exclusions (test files, generated files, vendor/).                                      |
| `--tokens`              | `4096`  | Target token budget for the output map.                                                                          |
| `--output`              | -       | Write map to a file instead of stdout.                                                                           |
| `--cache-dir`           | -       | Override the tag cache base directory. Default: OS cache dir (`~/.cache` on Linux, `~/Library/Caches` on macOS). |
| `--verbose`             | `false` | Print progress and scan info to stderr.                                                                          |

## Default exclusions

Goldfish automatically excludes the following by default - these are almost never useful in an LLM map:

**Test files:** `*_test.go`, `*.test.ts`, `*.test.js`, `*.test.tsx`, `*.test.jsx`, `*.spec.ts`, `*.spec.js`, `*.spec.tsx`, `*.spec.jsx`

**Test directories:** `__tests__/`, `testdata/`

**Generated files:** `*.pb.go`, `*.pb.gw.go`, `*.gen.go`, `*.generated.go`

**Dependency dirs:** `vendor/`

Override:

```bash
# Add an extra exclusion on top of defaults
goldfish --exclude "fixtures/**" /path/to/repo

# Include everything - disable all defaults
goldfish --no-default-excludes /path/to/repo

# Include tests but keep other defaults (no clean way - use --no-default-excludes and re-add what you want)
goldfish --no-default-excludes \
  --exclude "vendor/**" --exclude "dist/**" --exclude "node_modules/**" \
  /path/to/repo
```

## How it works

1. **Tag extraction** - tree-sitter parses every source file and extracts symbol definitions (`def`) and references (`ref`).
2. **Graph** - a weighted directed graph is built where edges connect files that reference each other's symbols. Cross-file references are weighted by frequency and naming style.
3. **PageRank** - files and symbols that are heavily referenced float to the top. `--chat` files apply a strong bias (50×), pulling their neighbours up in the ranking. `--mention-file` and `--mention-ident` apply a moderate boost - useful for hinting the current task context.
4. **Rendering** - for each top-ranked symbol, the AST is walked upward to find its enclosing scope (function signature, type declaration, etc.). Only the header line is shown; the body is collapsed to `⋮`.
5. **Token budget** - the ranked list is binary-searched to find the largest slice that fits within `--tokens`.

## Supported languages

| Language   | Extensions                           |
| ---------- | ------------------------------------ |
| Go         | `.go`                                |
| Python     | `.py`                                |
| JavaScript | `.js`, `.mjs`, `.cjs`, `.jsx`        |
| TypeScript | `.ts`                                |
| TSX        | `.tsx`                               |
| Rust       | `.rs`                                |
| Java       | `.java`                              |
| C          | `.c`, `.h`                           |
| C++        | `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh` |
| Ruby       | `.rb`                                |
| C#         | `.cs`                                |

Files with unsupported extensions are silently skipped. When scanning a directory, `.gitignore` rules are respected automatically.

## Output example

Important files (README, go.mod, Dockerfile, package.json, etc.) are always surfaced first, regardless of rank. Then ranked symbols follow - one signature per definition with `⋮` collapsing the body:

```
README.md

go.mod

auth/middleware.go:
func RequireAuth(next http.Handler) http.Handler {
⋮
func extractToken(r *http.Request) (string, error) {
⋮

store/user.go:
type UserStore struct {
⋮
func (s *UserStore) GetByID(id string) (*User, error) {
⋮
func (s *UserStore) Save(u *User) error {
⋮
```

Lines longer than 100 characters are truncated - guards against minified JS or generated code.

## Known limitations

- Token counting uses a character-based estimate (4 chars per token). Accurate enough for budget purposes but not exact.
- Files with no supported extension are silently skipped.
- Tag cache is stored at `~/Library/Caches/goldfish/` (macOS), `~/.cache/goldfish/` (Linux), or `%LOCALAPPDATA%/goldfish/` (Windows), keyed by repo path and file mtime. Safe to delete - it's a speed optimisation only.

## Development

See [CONTRIBUTING.md](./CONTRIBUTING.md) for setup, development commands, commit conventions, and how to add language support.

## License

MIT - see [LICENSE](./LICENSE). See [Credits](#credits) above for attribution of the ported algorithm and vendored query files.
