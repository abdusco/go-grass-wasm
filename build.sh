#!/usr/bin/env bash
set -euo pipefail

OUT_WASM="${1:-grass/grass.wasm}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

docker run --rm \
  -e OUT_WASM="$OUT_WASM" \
  -v "$PWD:/out" \
  debian:bookworm-slim \
  bash -lc '
    set -euo pipefail
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl build-essential binaryen
    rm -rf /var/lib/apt/lists/*

    curl https://sh.rustup.rs -sSf | sh -s -- -y --profile minimal
    . "$HOME/.cargo/env"

    cd /out/rust-shim

    rustup target add wasm32-wasip1
    cargo build --release --target wasm32-wasip1

    mkdir -p "$(dirname "/out/$OUT_WASM")"
    cp target/wasm32-wasip1/release/grass_wasm_shim.wasm "/out/$OUT_WASM"
  '

echo "Built $OUT_WASM"
