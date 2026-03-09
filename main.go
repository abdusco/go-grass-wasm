package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

func main() {
	in := flag.String("i", "-", "Input SCSS file path or - for stdin")
	out := flag.String("o", "-", "Output CSS file path or - for stdout")
	wasmPath := flag.String("wasm", "grass.wasm", "Path to grass.wasm")
	flag.Parse()

	css, stderrOut, err := compile(*wasmPath, *in)
	if err != nil {
		if stderrOut != "" {
			fmt.Fprint(os.Stderr, stderrOut)
		}
		fmt.Fprintf(os.Stderr, "go-grass: %v\n", err)
		os.Exit(1)
	}

	if *out == "-" {
		if _, err := os.Stdout.Write(css); err != nil {
			fmt.Fprintf(os.Stderr, "go-grass: writing stdout: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(*out, css, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "go-grass: writing %s: %v\n", *out, err)
		os.Exit(1)
	}
}

func compile(wasmPath, input string) ([]byte, string, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, "", fmt.Errorf("reading wasm module %q: %w", wasmPath, err)
	}

	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return nil, "", fmt.Errorf("instantiating WASI: %w", err)
	}

	compiled, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, "", fmt.Errorf("compiling wasm module: %w", err)
	}
	defer compiled.Close(ctx)

	var stdin io.Reader
	args := []string{"grass"}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("reading current directory: %w", err)
	}

	if input == "-" {
		args = append(args, "--stdin")
		stdin = os.Stdin
	} else {
		if filepath.IsAbs(input) {
			rel, err := filepath.Rel(cwd, input)
			if err != nil {
				return nil, "", fmt.Errorf("resolving input path %q: %w", input, err)
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, "", fmt.Errorf("input file %q is outside current directory %q", input, cwd)
			}
			input = rel
		}

		input = filepath.ToSlash(input)
		args = append(args, input)
	}

	fsConfig := wazero.NewFSConfig().WithDirMount(cwd, "/")

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	moduleConfig := wazero.NewModuleConfig().
		WithArgs(args...).
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithFSConfig(fsConfig)

	if stdin != nil {
		moduleConfig = moduleConfig.WithStdin(stdin)
	}

	_, err = runtime.InstantiateModule(ctx, compiled, moduleConfig)
	if err != nil {
		var exitErr *sys.ExitError
		if errors.As(err, &exitErr) {
			return nil, stderrBuf.String(), fmt.Errorf("grass exited with code %d", exitErr.ExitCode())
		}
		return nil, stderrBuf.String(), fmt.Errorf("running grass wasm: %w", err)
	}

	return stdoutBuf.Bytes(), stderrBuf.String(), nil
}
