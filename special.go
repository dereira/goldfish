package main

import (
	"path"
	"path/filepath"
	"strings"
)

// importantFiles is the set of filenames always surfaced at the top of the map
// regardless of their PageRank score.
var importantFiles = map[string]bool{
	".gitignore": true, ".gitattributes": true,
	"README": true, "README.md": true, "README.txt": true, "README.rst": true,
	"CONTRIBUTING": true, "CONTRIBUTING.md": true, "CONTRIBUTING.txt": true, "CONTRIBUTING.rst": true,
	"LICENSE": true, "LICENSE.md": true, "LICENSE.txt": true,
	"CHANGELOG": true, "CHANGELOG.md": true, "CHANGELOG.txt": true, "CHANGELOG.rst": true,
	"SECURITY": true, "SECURITY.md": true, "SECURITY.txt": true,
	"CODEOWNERS":       true,
	"requirements.txt": true, "Pipfile": true, "Pipfile.lock": true,
	"pyproject.toml": true, "setup.py": true, "setup.cfg": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true,
	"Gemfile": true, "Gemfile.lock": true,
	"composer.json": true, "composer.lock": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true,
	"go.mod": true, "go.sum": true,
	"Cargo.toml": true, "Cargo.lock": true,
	"mix.exs": true,
	".env":    true, ".env.example": true, ".editorconfig": true,
	"tsconfig.json": true, "jsconfig.json": true,
	".eslintrc": true, ".prettierrc": true,
	".pylintrc": true, ".flake8": true, "mypy.ini": true,
	"pyrightconfig.json": true,
	"Makefile":           true, "makefile": true,
	"webpack.config.js": true, "rollup.config.js": true,
	".travis.yml": true, ".gitlab-ci.yml": true, "Jenkinsfile": true,
	"azure-pipelines.yml": true, "appveyor.yml": true,
	"codecov.yml": true, ".coveragerc": true,
	"Dockerfile": true, "docker-compose.yml": true, "docker-compose.override.yml": true,
	"terraform.tf": true, "main.tf": true,
	"pytest.ini": true, "jest.config.js": true,
}

// isImportant reports whether a relative file path should be treated as
// structurally important (surfaced before ranked tags).
func isImportant(relPath string) bool {
	name := filepath.Base(relPath)
	dir := filepath.Dir(relPath)

	// GitHub Actions workflows aren't matched by filename alone - check the path.
	if strings.HasSuffix(name, ".yml") &&
		(dir == ".github/workflows" || dir == filepath.Join(".github", "workflows")) {
		return true
	}

	return importantFiles[name]
}

// filterImportant returns only the paths from relPaths that are important.
func filterImportant(relPaths []string) []string {
	out := make([]string, 0)
	for _, p := range relPaths {
		if isImportant(p) {
			out = append(out, p)
		}
	}
	return out
}

// defaultExcludes is the set of glob patterns applied by default to exclude
// test files, generated files, and vendored/built dependency directories.
var defaultExcludes = []string{
	"*_test.go",
	"*.test.ts", "*.test.js", "*.test.tsx", "*.test.jsx",
	"*.spec.ts", "*.spec.js", "*.spec.tsx", "*.spec.jsx",
	"__tests__/**",
	"testdata/**",
	"*.pb.go", "*.pb.gw.go",
	"*.gen.go", "*.generated.go",
	"vendor/**",
}

func shouldExclude(relPath string, patterns []string) bool {
	base := path.Base(relPath)
	for _, pattern := range patterns {
		// Recursive directory patterns (e.g. "vendor/**") are matched as a prefix
		// rather than via path.Match, which doesn't support **.
		if strings.HasSuffix(pattern, "/**") {
			dir := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(relPath, dir+"/") {
				return true
			}
			continue
		}
		if matched, err := path.Match(pattern, relPath); err == nil && matched {
			return true
		}
		// For patterns without a directory separator, also match against base name
		// so that e.g. "*.pb.go" excludes "api/service.pb.go".
		if !strings.Contains(pattern, "/") {
			if matched, err := path.Match(pattern, base); err == nil && matched {
				return true
			}
		}
	}
	return false
}
