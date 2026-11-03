#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ASSETS="${ASSETS:-$ROOT/assets}"
WIDTH="${WIDTH:-750}"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

AUSTIN_URL="${AUSTIN_URL:-https://github.com/AustinGabriel/Public-Domain-and-CC0-Playing-Cards/archive/refs/heads/main.zip}"
SAUL_URL="${SAUL_URL:-https://github.com/saulspatz/SVGCards/archive/refs/heads/master.zip}"
SEELIG_URL="${SEELIG_URL:-https://github.com/seeligd/svg-playing-cards/archive/refs/heads/main.zip}"

if ! command -v rsvg-convert >/dev/null; then
  echo "rsvg-convert est nécessaire (brew install librsvg)" >&2
  exit 1
fi

fetch() {
  curl -q -sSL --fail -o "$WORK_DIR/$1.zip" "$2"
  unzip -q -o "$WORK_DIR/$1.zip" -d "$WORK_DIR/$1"
}

fetch austin "$AUSTIN_URL"
fetch saul "$SAUL_URL"
fetch seelig "$SEELIG_URL"

rm -rf "$ASSETS"
mkdir -p "$ASSETS"
mv "$WORK_DIR/austin" "$ASSETS/austin"

ranks_saul=(Ace King Queen Jack 10 9 8 7)
ranks_seelig=(ace king queen jack 10 9 8 7)

for variant in Vertical2 Vertical4; do
  mkdir -p "$ASSETS/saul_png/$variant"
  for suit in club diamond heart spade; do
    for rank in "${ranks_saul[@]}"; do
      rsvg-convert -w "$WIDTH" "$WORK_DIR"/saul/*/Decks/"$variant"/svgs/"$suit$rank".svg -o "$ASSETS/saul_png/$variant/$suit$rank.png"
    done
  done
done

for font in abril roboto; do
  source_dir="$(echo "$WORK_DIR"/seelig/*)"
  [ "$font" = roboto ] && source_dir="$source_dir/roboto"

  mkdir -p "$ASSETS/seelig_png/$font"
  for suit in clubs diamonds hearts spades; do
    for rank in "${ranks_seelig[@]}"; do
      rsvg-convert -w "$WIDTH" "$source_dir/${rank}_of_${suit}.svg" -o "$ASSETS/seelig_png/$font/${rank}_of_${suit}.png"
    done
  done
done
