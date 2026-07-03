---
name: goldfish
description: 'MANDATORY: Run goldfish before reading any source files in an unfamiliar codebase. The output is a navigation target list - it tells you exactly which files and functions to open. Do not glob directories or read files speculatively; goldfish replaces that entirely. Required whenever: starting work on a new codebase, exploring an unfamiliar module, beginning a feature or bug fix in unknown territory, or any time you would otherwise start opening files to understand structure.'
---

# Goldfish - Repo Mapper

Goldfish produces a ranked list of the files and symbols that matter most in a codebase - ordered by cross-file importance, token-budgeted to fit your context. **Use the output as your navigation target list:** the file paths and function signatures that appear are where you go. Files that don't appear are not your concern for this task.

Do not read source files to understand a codebase. Run goldfish, get the target list, navigate to exactly what it surfaces.

Requires `goldfish` on PATH: `go install github.com/dereira/goldfish@latest`

## Call goldfish iteratively, not once

Don't try to get everything from a single invocation. Between calls, act on what surfaced: open the files and functions the map identified, work in them, then re-run with narrower flags as you learn more. Each call produces a tighter target list.

1. **Cold start** - `goldfish /path/to/repo` to identify the highest-ranked files and symbols. What surfaces here is where you start - open those files, not random ones.
2. **Narrow in** - once you have files in context, re-run with `--chat` on those files. Their neighbourhood rises; you get the callers, callees, and types you need next.
3. **Task-scope** - once you know what you're building or fixing, re-run with `--mention-file` and `--mention-ident` to pull the exact neighbourhood to the top.
4. **Budget up** - if the map feels thin, re-run with a higher `--tokens` value to surface more targets.

There is no reason to skip a refinement call - re-running goldfish with tighter flags costs far less than opening the wrong files.

## What to do with the map

The map output is a navigation decision, not background reading. Every file path and function signature that appears is a direct target:

- **Open exactly those files** - use the paths verbatim with `Read` or `lsp_goto_definition`.
- **Go to exactly those symbols** - navigate to the specific function or type, not a broad file scan.
- **Do not open files that did not appear in the map.** If a file wasn't surfaced, either it doesn't matter for the task, or you need to re-run with better `--mention-*` flags.

If you find yourself reading a file to understand what it does, stop. That's what goldfish is for. Re-run with `--mention-file` or `--mention-ident` targeting what you're looking for, then navigate to what surfaces.

## Core usage patterns

### 1. Map a repo cold (no task context yet)

```bash
goldfish /path/to/repo
```

Produces a 4096-token target list. What appears here is where you start.

### 2. You already have files open in the LLM context

Use `--chat` for files already loaded. They are **excluded from the map output** (no duplication) but their callers and callees are pulled up in ranking - you get the next ring of targets, not the files themselves.

```bash
goldfish --chat src/auth.go,src/middleware.go /path/to/repo
```

Multiple files: comma-separated. Spaces around commas are trimmed and safe. Paths resolved relative to CWD.

`--chat` files must be within the scanned path(s) to have any effect. Files outside the path are accepted without error but silently ignored for ranking.

### 3. You know what the task is about

Hint the ranker toward relevant files or symbols. These stay **in** the output but rank higher - they surface as primary targets.

```bash
# Boost a specific file
goldfish --mention-file src/payments/processor.go /path/to/repo

# Boost a specific symbol name
goldfish --mention-ident handleCheckout /path/to/repo

# Both flags are repeatable - stack as many as needed
goldfish \
  --mention-file src/payments/processor.go \
  --mention-file src/payments/refund.go \
  --mention-ident handleCheckout \
  --mention-ident processRefund \
  /path/to/repo
```

### 4. Combining chat + mentions (recommended for task-scoped targeting)

When you have files in context AND know the task - the tightest target list:

```bash
goldfish \
  --chat src/payments/processor.go \
  --mention-ident handleCheckout \
  --tokens 8192 \
  /path/to/repo
```

`--chat` biases the graph toward the neighbourhood of `processor.go` (50× weight on its edges). `--mention-ident` adds another 10× boost to `handleCheckout` specifically. Together they surface the callers, callees, and type dependencies that matter for this task - as a precise target list.

Note: adding `--mention-file` to a file already in `--chat` has no additive effect - `--chat` already provides the maximum bias.

### 5. Exclude noise

By default goldfish excludes test files, generated files, and vendored/built dirs. Add extra exclusions or disable the defaults:

```bash
# Add an extra exclusion on top of defaults
goldfish --exclude "fixtures/**" --exclude "mocks/**" /path/to/repo

# Disable all default exclusions (include test files, vendor, etc.)
goldfish --no-default-excludes /path/to/repo
```

**Default excluded patterns:** `*_test.go`, `*.test.ts/js/tsx/jsx`, `*.spec.ts/js`, `__tests__/**`, `testdata/**`, `*.pb.go`, `*.gen.go`, `*.generated.go`, `vendor/**`

### 6. Token budget

Default is 4096. Adjust to your model's available context:

```bash
goldfish --tokens 2000 /path/to/repo   # tight budget, fewest targets
goldfish --tokens 8192 /path/to/repo   # more targets, more context
goldfish --tokens 32000 /path/to/repo  # large context models
```

Token counting is character-based (4 chars ≈ 1 token) - accurate enough for budget purposes, not exact. Use at least 100 tokens; budgets below ~20 may produce no output and exit with: `error: no map generated - token budget too small (try increasing --tokens)`.

### 7. Debug / understand what's happening

```bash
goldfish --verbose /path/to/repo
```

Prints to stderr: root directory, files scanned, tags extracted, binary-search iterations.

## Reading the output

```
README.md                          ← always surfaced first (important file)
go.mod                             ← always surfaced first (important file)

src/auth/middleware.go:            ← open this file
func RequireAuth(next http.Handler) http.Handler {   ← go to this symbol
⋮                                  ← body omitted, not your concern yet
func extractToken(r *http.Request) (string, error) {
⋮

store/user.go:
type UserStore struct {
⋮
func (s *UserStore) GetByID(id string) (*User, error) {
⋮
```

**What `⋮` means:** one or more lines of body omitted. The signature immediately above it is always complete. Read the body only when you've navigated to that symbol and need to understand its implementation.

**Lines longer than 100 characters are truncated** - guards against minified JS or generated code polluting the map.

**Important files always appear first**, regardless of PageRank score: `README.md`, `go.mod`, `package.json`, `Dockerfile`, `Cargo.toml`, `pyproject.toml`, `tsconfig.json`, `.github/workflows/*.yml`, `Makefile`, `LICENSE`, and others.

**A file listed with no `:` and no snippet** means one of three things: it's a non-source important file (README, Makefile, go.mod - never parsed by tree-sitter); it's a source file that yielded no parseable definitions; or it ranked in the graph but all its definitions were outranked within the token budget.

## Supported languages and extensions

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

Files with unsupported extensions are **silently skipped**. Directories are walked recursively and `.gitignore` rules are respected automatically.

If goldfish finds **no supported source files** at the given path(s), it exits with: `error: no supported source files found`. This happens when the path contains only unsupported files (YAML, HCL, SQL, Markdown, etc.).

## Ranking mechanics (know this to use it well)

Goldfish runs PageRank on a weighted symbol graph. Understanding the weights helps you pick the right flags:

- **Cross-file references** create edges: file A references a symbol defined in file B → edge A→B weighted by reference frequency.
- **`--chat` files** get a **50× multiplier** on all their outgoing edges - their entire neighbourhood floats up. Use when a file is already in your LLM context.
- **`--mention-file`** sets personalization weight for a file - a moderate boost to its rank. Has no additive effect on a file already in `--chat`.
- **`--mention-ident`** boosts any symbol matching that name by **10×** in the graph and in the final scoring pass. Also boosts files whose path components (directory names or filename stems) match the identifier - so `--mention-ident payments` would boost `src/payments/processor.go` as well.
- **Meaningful identifiers rank higher:** symbols ≥8 characters with snake_case, camelCase, or kebab-case get a 10× multiplier. Single-letter and short generic names are naturally suppressed.
- **Private/underscore-prefixed symbols** (`_foo`) get a 0.1× penalty.
- **Symbols defined in more than 5 files** (e.g. `init`, `main`, `new`) get a 0.1× penalty - they are noise.
- **Single-file or ref-free repos:** if no cross-file references exist, goldfish falls back to treating each definition as self-referencing. Ranking still works but reflects definition density rather than cross-file connectivity.

Use `--mention-ident` with full, meaningful symbol names. `handleCheckout` will boost strongly; `h` or `run` will not.

## Cache

Goldfish caches parsed tags per file keyed by mtime. Modified files are re-parsed automatically on the next run - the cache is always consistent with the source. Cache location:

- macOS: `~/Library/Caches/goldfish/<repo-hash>/`
- Linux: `~/.cache/goldfish/<repo-hash>/` (or `$XDG_CACHE_HOME`)
- Windows: `%LOCALAPPDATA%/goldfish/<repo-hash>/`

Override with `--cache-dir /some/path`. The cache is safe to delete - subsequent runs re-parse from source.

## Multiple paths

Goldfish accepts multiple paths and/or individual files:

```bash
# Two subdirectories
goldfish src/ lib/

# A mix of files and directories
goldfish main.go src/ internal/

# Specific files only
goldfish src/auth.go src/store.go src/models.go
```

The common ancestor directory is computed automatically and used as the root for relative paths in the output.

## When not to use goldfish

- The repo contains only unsupported files (YAML, HCL, SQL, Terraform) - goldfish will exit with an error.
- You have ≤3 files to understand - read them directly.
- You already have the full codebase in context - use `--chat` on your open files instead.
