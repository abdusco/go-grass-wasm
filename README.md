# go-grass

A small CLI wrapper that runs the Rust [grass](https://github.com/connorskees/grass) Sass compiler as WASM using [wazero](https://github.com/tetratelabs/wazero).

## What this does

- Builds `grass` as a `wasm32-wasip1` module in Docker (no host Rust toolchain needed).
- Runs that WASM module from Go.
- Supports file and stdin input, and file or stdout output.

## Requirements

- Docker
- Go 1.22+

## Build the WASM module

```bash
./build.sh
```

This writes `grass.wasm` to the current directory.

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
```

Optional WASM path override:

```bash
./go-grass -wasm grass.wasm -i styles.scss -o -
```

## Flags

- `-i` input path or `-` for stdin (default: `-`)
- `-o` output path or `-` for stdout (default: `-`)
- `-wasm` path to wasm module (default: `grass.wasm`)

## Notes

- In file-input mode, the current working directory is mounted into the WASI filesystem so Sass imports work normally.
- Absolute input paths outside the current working directory are rejected.
- Compiler/runtime errors are printed to stderr and return a non-zero exit code.

## Quick test

```bash
printf '$c: #333;\n.a { color: $c; }\n' | ./go-grass -i - -o -
```
