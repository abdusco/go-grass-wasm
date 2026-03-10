package grass

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCompilePathWithMemoryFS(t *testing.T) {
	ctx := context.Background()
	compiler, err := NewCompiler(ctx)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	defer compiler.Close(ctx)

	mem := fstest.MapFS{
		"styles/main.scss":    &fstest.MapFile{Data: []byte(`@use "../shared/tokens"; .a { color: tokens.$c; }`)},
		"shared/_tokens.scss": &fstest.MapFile{Data: []byte(`$c: #123456;`)},
	}

	css, err := compiler.CompilePath(ctx, "/styles/main.scss", Options{FS: mem})
	if err != nil {
		t.Fatalf("CompilePath: %v", err)
	}

	got := string(css)
	if !strings.Contains(got, "#123456") {
		t.Fatalf("expected compiled css to include #123456, got %q", got)
	}
}

func TestCompileStringWithMemoryFSIncludeDir(t *testing.T) {
	ctx := context.Background()
	compiler, err := NewCompiler(ctx)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	defer compiler.Close(ctx)

	mem := fstest.MapFS{
		"shared/_tokens.scss": &fstest.MapFile{Data: []byte(`$c: #0f0;`)},
	}

	css, err := compiler.CompileString(ctx, `@use "tokens"; .a { color: tokens.$c; }`, Options{
		FS:          mem,
		IncludeDirs: []string{"/shared"},
	})
	if err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := string(css)
	if !strings.Contains(got, "#0f0") {
		t.Fatalf("expected compiled css to include #0f0, got %q", got)
	}
}

func TestCompilePathWithMemoryFSStyleFileImportsSubdir(t *testing.T) {
	ctx := context.Background()
	compiler, err := NewCompiler(ctx)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	defer compiler.Close(ctx)

	mem := fstest.MapFS{
		"styles/style.scss":          &fstest.MapFile{Data: []byte(`@use "./subdir/tokens"; .button { color: tokens.$brand; }`)},
		"styles/subdir/_tokens.scss": &fstest.MapFile{Data: []byte(`$brand: #1a2b3c;`)},
	}

	css, err := compiler.CompilePath(ctx, "/styles/style.scss", Options{FS: mem})
	if err != nil {
		t.Fatalf("CompilePath: %v", err)
	}

	got := string(css)
	if !strings.Contains(got, ".button") {
		t.Fatalf("expected compiled css to include .button, got %q", got)
	}
	if !strings.Contains(got, "#1a2b3c") {
		t.Fatalf("expected compiled css to include #1a2b3c, got %q", got)
	}
}
