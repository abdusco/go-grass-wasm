# go-grass

A small CLI wrapper that runs the Rust [grass](https://github.com/connorskees/grass) Sass compiler as WASM using [wazero](https://github.com/tetratelabs/wazero).

It also includes a reusable Go library package at `grass` (import path `github.com/abdusco/go-grass-wasm/grass`).

## What this does

- Builds a tiny Rust WASM shim (backed by `grass_compiler`) in Docker.
- Runs that WASM module from Go.
- Supports file and stdin input, and file or stdout output.
- Exposes a Go `Options` type for supported compiler options.

## Requirements

- Docker
- Go 1.22+

## Build the WASM module

```bash
./build.sh
```

This writes an embeddable module to `grass/grass.wasm` by default.

Optional output filename:

```bash
./build.sh my-grass.wasm
```

## Build the CLI

```bash
go build -o go-grass .
```

## Usage

```bash
./go-grass -i - -o -
./go-grass -i styles.scss -o styles.css
./go-grass -i styles.scss -o styles.css --style compressed
./go-grass -i styles.scss -o styles.css --include-dir node_modules/susy/sass
```

## Flags

- `-i` input path or `-` for stdin (default: `-`)
- `-o` output path or `-` for stdout (default: `-`)
- `--style` output style: `expanded` or `compressed` (default: `expanded`)
- `--include-dir` import directory; repeat this flag for multiple directories

Only `--style` and `--include-dir` are exposed as grass compiler options in the CLI.

## Go library

```go
package main

import (
  "context"
  "fmt"

  "github.com/abdusco/go-grass-wasm/grass"
)

func main() {
  ctx := context.Background()

  compiler, err := grass.NewCompiler(ctx)
  if err != nil {
    panic(err)
  }
  defer compiler.Close(ctx)

  css, err := compiler.CompileString(ctx, "$c: #333; .a { color: $c; }", grass.Options{
    Style: grass.StyleCompressed,
    IncludeDirs: []string{"./styles", "./node_modules"},
  })
  if err != nil {
    panic(err)
  }

  fmt.Println(string(css))
}
```

Supported library options (`grass.Options`):

- `Style` (`expanded` or `compressed`)
- `IncludeDirs` (mapped to grass `-I` load paths)
- `FS` (`fs.FS` mount for virtual/memory filesystems; read-only)

`unicode_error_messages` remains enabled by default.

Main library methods:

- `CompilePath(ctx, path, opts)` for file-based, multi-file Sass projects.
- `CompileString(ctx, source, opts)` for in-memory source input.

When `Options.FS` is set:

- `CompilePath` and `IncludeDirs` are treated as guest paths (for example `/styles/main.scss`, `/styles`).
- The mounted filesystem is exposed read-only to WASM.

## Notes

- In file-input mode, the input file directory and each `--include-dir` are mounted into the WASI filesystem.
- Compiler/runtime errors are printed to stderr and return a non-zero exit code.

## Quick test

```bash
printf '$c: #333;\n.a { color: $c; }\n' | ./go-grass -i - -o -
```
