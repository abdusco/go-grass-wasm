#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

OUT_WASM="${1:-grass/grass.wasm}"
IMAGE_TAG="${GRASS_WASM_IMAGE:-grass-wasm-shim}"
DOCKER_BUILD_FLAGS="${DOCKER_BUILD_FLAGS:-}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if [ -n "$DOCKER_BUILD_FLAGS" ]; then
  docker buildx build $DOCKER_BUILD_FLAGS -t "$IMAGE_TAG" rust-shim
else
  docker build -t "$IMAGE_TAG" rust-shim
fi

docker run --rm \
  -e OUT_WASM="$OUT_WASM" \
  -v "$PWD:/out" \
  "$IMAGE_TAG" \
  sh -lc '
    set -e
    mkdir -p "$(dirname "/out/$OUT_WASM")"
    cp /src/target/wasm32-wasip1/release/grass_wasm_shim.wasm "/out/$OUT_WASM"
  '

echo "Built $OUT_WASM"
