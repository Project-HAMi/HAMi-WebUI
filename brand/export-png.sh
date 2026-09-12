#!/bin/sh
set -eu

brand_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if ! command -v rsvg-convert >/dev/null 2>&1; then
  printf '%s\n' 'Install librsvg (rsvg-convert) to regenerate brand PNGs.' >&2
  exit 1
fi

mkdir -p "$brand_dir/png"

for variant in light dark; do
  for height in 64 128 256; do
    rsvg-convert --format png --height "$height" \
      --output "$brand_dir/png/hami-webui-horizontal-$variant-${height}h.png" \
      "$brand_dir/svg/hami-webui-horizontal-$variant.svg"
  done
done

# The original mark is 111.018937 units wide and 128 units high.
# Center it in a square canvas without stretching or changing its path.
for size in 32 64 128 256; do
  left=$(awk -v size="$size" 'BEGIN { printf "%.9f", size * (1 - 111.018937 / 128) / 2 }')
  rsvg-convert --format png --height "$size" \
    --page-width "$size" --page-height "$size" --left "$left" \
    --output "$brand_dir/png/hami-mark-$size.png" \
    "$brand_dir/svg/hami-mark.svg"
done

printf '%s\n' 'Regenerated brand/png from brand/svg.'
