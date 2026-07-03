# Contributing to goldfish

## Prerequisites

- Go (version pinned in [`go.mod`](./go.mod))
- A C compiler on `PATH` (tree-sitter's Go bindings require CGO — `CGO_ENABLED` is always `1` for this project, see [Taskfile.yml](./Taskfile.yml))
- [`task`](https://taskfile.dev/installation/) installed and on `PATH` (e.g. `brew install go-task`)

## Setup

```bash
git clone https://github.com/dereira/goldfish
cd goldfish
task build
```

## Development commands

```bash
task test            # run all tests with race detection
task test:cover      # run tests and open HTML coverage report
task lint            # run golangci-lint
task lint:fix        # run golangci-lint with auto-fix
task build           # build the binary to dist/goldfish
task tidy            # tidy go.mod / go.sum
task clean           # remove built binary and tag cache
```

## Commit messages: Conventional Commits required

This repo uses [release-please](https://github.com/googleapis/release-please) for versioning and changelogs, driven entirely by commit message prefixes. PRs that don't follow this format won't be picked up correctly:

- `feat: ...` — new feature (minor version bump)
- `fix: ...` — bug fix (patch version bump)
- `feat!: ...` / `fix!: ...` — breaking change (major version bump)
- `chore: ...`, `docs: ...`, `refactor: ...`, `test: ...` — no version bump, still shows in changelog grouping

If a PR contains multiple logical changes, prefer multiple commits with the right prefixes over one vague one.

## Adding a new language

Support for a language touches three places — missing one causes silent partial breakage (see the TSX scope-header gap below for a real example of what that looks like):

1. **`langs.go`** — register the tree-sitter grammar in `registry`, map file extensions in `extToLang`
2. **`queries/<name>-tags.scm`** — the tag-extraction query file. Prefer adapting [aider's vendored queries](https://github.com/Aider-AI/aider/tree/main/aider/queries/tree-sitter-language-pack) for the same language over writing one from scratch — see [Credits](./README.md#credits) for the attribution this requires
3. **`context.go`** — add a `scopeTypes` entry for the language's AST node kinds (function/class/interface declarations), so ranked symbols render with their enclosing signature instead of a bare line

After adding a language, add a fixture-based test case in `tags_test.go` and/or `context_test.go` covering at least one definition and one reference.

## Before opening a PR

- `task test` and `task lint` both pass
- New language support includes a `scopeTypes` entry — nothing silently degrades the way TSX briefly did
- Commit messages follow Conventional Commits

## License

By contributing, you agree your contributions are licensed under the project's [MIT License](./LICENSE).
