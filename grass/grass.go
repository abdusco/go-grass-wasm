package grass

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type Style string

const (
	StyleExpanded   Style = "expanded"
	StyleCompressed Style = "compressed"
)

type Options struct {
	Style       Style
	IncludeDirs []string
}

type CompileError struct {
	ExitCode uint32
	Stderr   string
}

func (e *CompileError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("grass exited with code %d: %s", e.ExitCode, strings.TrimSpace(e.Stderr))
	}
	return fmt.Sprintf("grass exited with code %d", e.ExitCode)
}

type Compiler struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

//go:embed grass.wasm
var grassWASM []byte

func NewCompiler(ctx context.Context) (*Compiler, error) {
	if len(grassWASM) == 0 {
		return nil, fmt.Errorf("embedded grass.wasm is empty")
	}

	runtime := wazero.NewRuntime(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("instantiating WASI: %w", err)
	}

	compiled, err := runtime.CompileModule(ctx, grassWASM)
	if err != nil {
		_ = runtime.Close(ctx)
		return nil, fmt.Errorf("compiling wasm module: %w", err)
	}

	return &Compiler{runtime: runtime, compiled: compiled}, nil
}

func (c *Compiler) Close(ctx context.Context) error {
	err1 := c.compiled.Close(ctx)
	err2 := c.runtime.Close(ctx)
	if err1 != nil {
		return err1
	}
	return err2
}

func (c *Compiler) CompileFile(ctx context.Context, inputPath string, options Options) ([]byte, error) {
	runCfg, stderrBuf, err := c.newModuleConfig(inputPath, nil, options)
	if err != nil {
		return nil, err
	}

	_, err = c.runtime.InstantiateModule(ctx, c.compiled, runCfg.config)
	if err != nil {
		return nil, decorateRunError(err, stderrBuf)
	}

	return runCfg.stdoutBuf.Bytes(), nil
}

func (c *Compiler) CompileString(ctx context.Context, source string, options Options) ([]byte, error) {
	runCfg, stderrBuf, err := c.newModuleConfig("", strings.NewReader(source), options)
	if err != nil {
		return nil, err
	}

	_, err = c.runtime.InstantiateModule(ctx, c.compiled, runCfg.config)
	if err != nil {
		return nil, decorateRunError(err, stderrBuf)
	}

	return runCfg.stdoutBuf.Bytes(), nil
}

type runConfig struct {
	config    wazero.ModuleConfig
	stdoutBuf *bytes.Buffer
	stderrBuf *bytes.Buffer
}

func (c *Compiler) newModuleConfig(inputPath string, stdin *strings.Reader, options Options) (runConfig, *bytes.Buffer, error) {
	args, fsConfig, err := buildArgsAndFS(inputPath, options)
	if err != nil {
		return runConfig{}, nil, err
	}

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	moduleConfig := wazero.NewModuleConfig().
		WithArgs(args...).
		WithStdout(stdoutBuf).
		WithStderr(stderrBuf).
		WithFSConfig(fsConfig)

	if stdin != nil {
		moduleConfig = moduleConfig.WithStdin(stdin)
	}

	return runConfig{config: moduleConfig, stdoutBuf: stdoutBuf, stderrBuf: stderrBuf}, stderrBuf, nil
}

func buildArgsAndFS(inputPath string, options Options) ([]string, wazero.FSConfig, error) {
	style := options.Style
	if style == "" {
		style = StyleExpanded
	}
	if style != StyleExpanded && style != StyleCompressed {
		return nil, nil, fmt.Errorf("invalid style %q", style)
	}

	args := []string{"grass", "--style", string(style)}
	fsConfig := wazero.NewFSConfig()
	mountIdx := 0

	mountDir := func(host string) (string, error) {
		abs, err := filepath.Abs(host)
		if err != nil {
			return "", err
		}
		guest := fmt.Sprintf("/m%d", mountIdx)
		mountIdx++
		fsConfig = fsConfig.WithDirMount(abs, guest)
		return guest, nil
	}

	for _, includeDir := range options.IncludeDirs {
		guest, err := mountDir(includeDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving include dir %q: %w", includeDir, err)
		}
		args = append(args, "-I", guest)
	}

	if inputPath == "" {
		args = append(args, "--stdin")
	} else {
		absInput, err := filepath.Abs(inputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving input path %q: %w", inputPath, err)
		}
		guestDir, err := mountDir(filepath.Dir(absInput))
		if err != nil {
			return nil, nil, fmt.Errorf("mounting input dir %q: %w", filepath.Dir(absInput), err)
		}
		args = append(args, guestDir+"/"+filepath.Base(absInput))
	}

	return args, fsConfig, nil
}

func decorateRunError(err error, stderrBuf *bytes.Buffer) error {
	var exitErr *sys.ExitError
	if errors.As(err, &exitErr) {
		return &CompileError{ExitCode: exitErr.ExitCode(), Stderr: stderrBuf.String()}
	}

	if stderrBuf.Len() > 0 {
		return fmt.Errorf("running grass wasm: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	return fmt.Errorf("running grass wasm: %w", err)
}
