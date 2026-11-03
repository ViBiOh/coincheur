#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEIGHTS="${WEIGHTS:-$ROOT/models/corners.pt}"
MODELS_DIR="${MODELS_DIR:-$ROOT/build/models}"
IMAGE_SIZE="${IMAGE_SIZE:-832}"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

cp "$WEIGHTS" "$WORK_DIR/corners.pt"

(
  cd "$WORK_DIR"
  uv run --no-project --with ultralytics --with onnx --with onnxslim python -c \
    "from ultralytics import YOLO; YOLO('corners.pt').export(format='onnx', imgsz=$IMAGE_SIZE, opset=12)"
)

mkdir -p "$MODELS_DIR"
mv "$WORK_DIR/corners.onnx" "$MODELS_DIR/corners.onnx"
