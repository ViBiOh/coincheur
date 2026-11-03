#!/usr/bin/env bash
set -euo pipefail

ORT_VERSION="${ORT_VERSION:-1.31.0}"
ORT_URL="${ORT_URL:-https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-osx-arm64-${ORT_VERSION}.tgz}"
ORT_SHA256="${ORT_SHA256:-1031b36dd3bdaa8d976a6130d9cf2a05e9be320b93ae804f8427b91dc3432afc}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIB_DIR="${LIB_DIR:-$ROOT/build/lib}"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

curl -q -sSL --fail -o "$WORK_DIR/ort.tgz" "$ORT_URL"

actual="$(shasum -a 256 "$WORK_DIR/ort.tgz" | cut -d' ' -f1)"
if [ "$actual" != "$ORT_SHA256" ]; then
  echo "Checksum invalide: $actual (attendu $ORT_SHA256)" >&2
  exit 1
fi

tar xzf "$WORK_DIR/ort.tgz" -C "$WORK_DIR"

mkdir -p "$LIB_DIR"
cp -L "$WORK_DIR"/onnxruntime-*/lib/libonnxruntime.dylib "$LIB_DIR/libonnxruntime.dylib"
