package grass

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed grass.wasm
var grassWASM []byte

type Style int32

const (
	StyleExpanded Style = iota
	StyleCompressed
)

type Options struct {
	Style       Style
	IncludeDirs []string
	FS          fs.FS
}

type CompileError struct {
	Stderr string
}

func (e *CompileError) Error() string {
	if e.Stderr == "" {
		return "compilation failed"
	}
	return strings.TrimSpace(e.Stderr)
}

type Compiler struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

func NewCompiler(ctx context.Context) (*Compiler, error) {
	if len(grassWASM) == 0 {
		return nil, fmt.Errorf("embedded grass.wasm is empty")
	}

	runtime := wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("instantiate WASI: %w", err)
	}

	compiled, err := runtime.CompileModule(ctx, grassWASM)
	if err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("compile wasm module: %w", err)
	}

	return &Compiler{runtime: runtime, compiled: compiled}, nil
}

func (c *Compiler) Close(ctx context.Context) error {
	errA := c.compiled.Close(ctx)
	errB := c.runtime.Close(ctx)
	if errA != nil {
		return errA
	}
	return errB
}

func (c *Compiler) CompilePath(ctx context.Context, path string, opts Options) ([]byte, error) {
	run, err := c.newRun(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer run.mod.Close(ctx)

	normalizedPath, err := normalizeInputPath(path, opts.FS != nil)
	if err != nil {
		return nil, fmt.Errorf("resolve path %q: %w", path, err)
	}

	pathBytes := []byte(normalizedPath)
	pathPtr, err := run.writeBytes(ctx, pathBytes)
	if err != nil {
		return nil, err
	}
	defer run.freeBytes(ctx, pathPtr, uint64(len(pathBytes)))

	includeDirs, err := normalizeIncludeDirs(opts.IncludeDirs, opts.FS != nil)
	if err != nil {
		return nil, err
	}

	incBytes := []byte(strings.Join(includeDirs, "\n"))
	incPtr, err := run.writeBytes(ctx, incBytes)
	if err != nil {
		return nil, err
	}
	defer run.freeBytes(ctx, incPtr, uint64(len(incBytes)))

	style := normalizeStyle(opts.Style)
	status, err := run.call1(ctx, "compile_path", pathPtr, uint64(len(pathBytes)), uint64(style), incPtr, uint64(len(incBytes)))
	if err != nil {
		return nil, err
	}

	if status != 0 {
		stderr, readErr := run.readResult(ctx, "get_error_ptr", "get_error_len")
		if readErr != nil {
			return nil, readErr
		}
		return nil, &CompileError{Stderr: string(stderr)}
	}

	return run.readResult(ctx, "get_output_ptr", "get_output_len")
}

func (c *Compiler) CompileString(ctx context.Context, source string, opts Options) ([]byte, error) {
	run, err := c.newRun(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer run.mod.Close(ctx)

	srcBytes := []byte(source)
	srcPtr, err := run.writeBytes(ctx, srcBytes)
	if err != nil {
		return nil, err
	}
	defer run.freeBytes(ctx, srcPtr, uint64(len(srcBytes)))

	includeDirs, err := normalizeIncludeDirs(opts.IncludeDirs, opts.FS != nil)
	if err != nil {
		return nil, err
	}

	incBytes := []byte(strings.Join(includeDirs, "\n"))
	incPtr, err := run.writeBytes(ctx, incBytes)
	if err != nil {
		return nil, err
	}
	defer run.freeBytes(ctx, incPtr, uint64(len(incBytes)))

	style := normalizeStyle(opts.Style)
	status, err := run.call1(ctx, "compile_string", srcPtr, uint64(len(srcBytes)), uint64(style), incPtr, uint64(len(incBytes)))
	if err != nil {
		return nil, err
	}

	if status != 0 {
		stderr, readErr := run.readResult(ctx, "get_error_ptr", "get_error_len")
		if readErr != nil {
			return nil, readErr
		}
		return nil, &CompileError{Stderr: string(stderr)}
	}

	return run.readResult(ctx, "get_output_ptr", "get_output_len")
}

func normalizeStyle(style Style) Style {
	if style == StyleCompressed {
		return StyleCompressed
	}
	return StyleExpanded
}

type wasmRun struct {
	mod     api.Module
	memory  api.Memory
	alloc   api.Function
	dealloc api.Function
}

func (c *Compiler) newRun(ctx context.Context, opts Options) (*wasmRun, error) {
	fsConfig := wazero.NewFSConfig()
	if opts.FS != nil {
		fsConfig = fsConfig.WithFSMount(newReadOnlyFS(opts.FS), "/")
	} else {
		fsConfig = fsConfig.WithDirMount("/", "/")
	}

	mod, err := c.runtime.InstantiateModule(ctx, c.compiled, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	if err != nil {
		return nil, fmt.Errorf("instantiate wasm module: %w", err)
	}

	run := &wasmRun{mod: mod, memory: mod.Memory()}
	if run.memory == nil {
		_ = mod.Close(ctx)
		return nil, fmt.Errorf("wasm module has no exported memory")
	}

	run.alloc = mod.ExportedFunction("alloc")
	run.dealloc = mod.ExportedFunction("dealloc")
	if run.alloc == nil || run.dealloc == nil {
		_ = mod.Close(ctx)
		return nil, fmt.Errorf("missing alloc/dealloc exports")
	}

	return run, nil
}

func (r *wasmRun) call1(ctx context.Context, name string, params ...uint64) (uint64, error) {
	fn := r.mod.ExportedFunction(name)
	if fn == nil {
		return 0, fmt.Errorf("missing export %q", name)
	}

	results, err := fn.Call(ctx, params...)
	if err != nil {
		return 0, fmt.Errorf("call %s: %w", name, err)
	}
	if len(results) == 0 {
		return 0, nil
	}
	return results[0], nil
}

func (r *wasmRun) writeBytes(ctx context.Context, data []byte) (uint64, error) {
	if len(data) == 0 {
		return 0, nil
	}

	results, err := r.alloc.Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("alloc: %w", err)
	}
	ptr := uint32(results[0])
	if ok := r.memory.Write(ptr, data); !ok {
		return 0, fmt.Errorf("write memory failed")
	}
	return uint64(ptr), nil
}

func (r *wasmRun) freeBytes(ctx context.Context, ptr uint64, length uint64) {
	if ptr == 0 || length == 0 {
		return
	}
	_, _ = r.dealloc.Call(ctx, ptr, length)
}

func (r *wasmRun) readResult(ctx context.Context, ptrFn, lenFn string) ([]byte, error) {
	ptr, err := r.call1(ctx, ptrFn)
	if err != nil {
		return nil, err
	}
	length, err := r.call1(ctx, lenFn)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return []byte{}, nil
	}

	view, ok := r.memory.Read(uint32(ptr), uint32(length))
	if !ok {
		return nil, errors.New("read memory failed")
	}
	out := make([]byte, len(view))
	copy(out, view)
	return out, nil
}

func normalizeIncludeDirs(includeDirs []string, useGuestPaths bool) ([]string, error) {
	out := make([]string, 0, len(includeDirs))
	for _, dir := range includeDirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}

		if useGuestPaths {
			out = append(out, normalizeGuestPath(trimmed))
			continue
		}

		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("resolve include-dir %q: %w", dir, err)
		}
		out = append(out, filepath.ToSlash(abs))
	}
	return out, nil
}

func normalizeInputPath(input string, useGuestPath bool) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("path is empty")
	}

	if useGuestPath {
		return normalizeGuestPath(trimmed), nil
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(abs), nil
}

func normalizeGuestPath(p string) string {
	guest := filepath.ToSlash(strings.TrimSpace(p))
	if !strings.HasPrefix(guest, "/") {
		guest = "/" + guest
	}
	return path.Clean(guest)
}

type readOnlyFS struct {
	inner fs.FS
}

func newReadOnlyFS(inner fs.FS) fs.FS {
	return &readOnlyFS{inner: inner}
}

func (r *readOnlyFS) Open(name string) (fs.File, error) {
	f, err := r.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &readOnlyFile{inner: f}, nil
}

type readOnlyFile struct {
	inner fs.File
}

func (f *readOnlyFile) Stat() (fs.FileInfo, error) {
	return f.inner.Stat()
}

func (f *readOnlyFile) Read(p []byte) (int, error) {
	return f.inner.Read(p)
}

func (f *readOnlyFile) Close() error {
	return f.inner.Close()
}

func (f *readOnlyFile) ReadDir(n int) ([]fs.DirEntry, error) {
	reader, ok := f.inner.(fs.ReadDirFile)
	if !ok {
		return nil, fs.ErrInvalid
	}
	return reader.ReadDir(n)
}
