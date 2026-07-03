package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTagCache_StoreAndLoad(t *testing.T) {
	dir := t.TempDir()
	cache := &tagCache{dir: filepath.Join(dir, "cache")}

	fname := filepath.Join(dir, "file.go")
	mtime := 1234567890.5
	tags := []Tag{
		{RelFname: "file.go", Fname: fname, Line: 0, Name: "Foo", Kind: kindDef},
		{RelFname: "file.go", Fname: fname, Line: 5, Name: "Bar", Kind: kindRef},
	}

	// Store then load - should get the same tags back.
	cache.store(fname, mtime, tags)
	got, ok := cache.load(fname, mtime)
	if !ok {
		t.Fatal("load() returned ok=false after store()")
	}
	if len(got) != len(tags) {
		t.Fatalf("load() returned %d tags; want %d", len(got), len(tags))
	}
	for i, tag := range tags {
		if got[i].Name != tag.Name || got[i].Kind != tag.Kind {
			t.Errorf("tag[%d]: got {Name:%q Kind:%q}; want {Name:%q Kind:%q}",
				i, got[i].Name, got[i].Kind, tag.Name, tag.Kind)
		}
	}
}

func TestTagCache_LoadMiss_NoFile(t *testing.T) {
	dir := t.TempDir()
	cache := &tagCache{dir: filepath.Join(dir, "cache")}

	_, ok := cache.load("/nonexistent/file.go", 123.0)
	if ok {
		t.Error("load() returned ok=true for non-existent cache entry")
	}
}

func TestTagCache_LoadMiss_MtimeMismatch(t *testing.T) {
	dir := t.TempDir()
	cache := &tagCache{dir: filepath.Join(dir, "cache")}

	fname := filepath.Join(dir, "file.go")
	tags := []Tag{{RelFname: "file.go", Name: "Foo", Kind: kindDef}}

	cache.store(fname, 1000.0, tags)

	// Same file, different mtime - should be a cache miss.
	_, ok := cache.load(fname, 9999.0)
	if ok {
		t.Error("load() returned ok=true when mtime does not match stored value")
	}
}

func TestTagCache_LoadMiss_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	cache := &tagCache{dir: cacheDir}

	fname := filepath.Join(dir, "file.go")

	// Write corrupt JSON directly to the cache key path.
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(cache.keyPath(fname), []byte("{invalid json"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, ok := cache.load(fname, 1000.0)
	if ok {
		t.Error("load() returned ok=true for corrupt JSON cache entry")
	}
}

func TestTagCache_StoreUnwritableDir(t *testing.T) {
	dir := t.TempDir()

	// Create cache dir and make it read-only so MkdirAll inside store() fails.
	cacheDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(cacheDir, 0o400); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() {
		err := os.Chmod(cacheDir, 0o700)
		if err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}) // restore so TempDir cleanup works

	cache := &tagCache{dir: filepath.Join(cacheDir, "nested")}
	fname := filepath.Join(dir, "file.go")

	// store() should fail silently - no panic, no return value.
	cache.store(fname, 1000.0, []Tag{{Name: "Foo", Kind: kindDef}})
}

// TestGenerate_CacheHit verifies that running Generate twice on the same file
// produces the same output - confirming the cache round-trip works end-to-end.
func TestGenerate_CacheHit(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "main.go")
	writeFile(t, fpath, `package main

func CachedFunc() int {
	return 1
}
`)

	first, err := Generate([]string{fpath}, nil, nil, nil, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}

	second, err := Generate([]string{fpath}, nil, nil, nil, nil, "", 2000, false)
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}

	if first != second {
		t.Errorf("cache hit produced different output\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
