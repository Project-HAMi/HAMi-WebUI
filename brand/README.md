# HAMi WebUI brand assets

This directory is the shared source of the HAMi WebUI identity for the
application, documentation, presentations, and community materials.
Use the SVG masters whenever possible; PNG exports are provided for tools
that need raster images.

<picture>
  <source media="(prefers-color-scheme: dark)"
    srcset="svg/hami-webui-horizontal-dark.svg">
  <img src="svg/hami-webui-horizontal-light.svg" alt="HAMi WebUI" width="320">
</picture>

## Choose an asset

| Asset | Intended use |
| --- | --- |
| [Horizontal, light background](svg/hami-webui-horizontal-light.svg) | Light application surfaces, README files, documents, and slides |
| [Horizontal, dark background](svg/hami-webui-horizontal-dark.svg) | Dark surfaces, with light lettering |
| [HAMi melon mark](svg/hami-mark.svg) | Compact placements where HAMi WebUI is already identified, such as collapsed navigation |
| [Transparent PNG exports](png/) | Documents and presentation tools that cannot use SVG |

The `light` and `dark` suffixes describe the **background** the artwork is
designed for. The files do not change color based on system preferences.
Choose the asset for the actual surface; a light application sidebar still
uses the light-background version when the operating system has a dark theme.

All SVGs are self-contained vectors with no font or raster dependencies.
The full horizontal SVG retains the melon, original HAMi lettering, and
smaller WebUI companion lettering as one asset.

```text
brand/
  README.md
  svg/                    # Canonical vector sources
  png/                    # Generated transparent exports
  export-png.sh           # Rebuild the PNGs from the SVGs
```

## Size, spacing, and color

Preserve the artwork's aspect ratio. Do not stretch, rotate, crop, recolor,
add shadows, or replace the outlined lettering with a font. Place the mark
on a quiet background, away from surrounding text and controls.

| Placement | Recommended size |
| --- | --- |
| Expanded application navigation | 32px high; the artwork is 145.5px wide inside a 148px link |
| Collapsed application navigation | A 32px square slot, with the melon centered at its original aspect ratio |
| README and document header | 300–320px wide; use SVG for sharp lettering |
| Compact use | Use the melon mark when the full identity would be smaller than 32px high |

Keep clear space of at least one quarter of the artwork height on every
side: 8px around the 32px application logo, or approximately 18px around a
320px-wide document logo. This space belongs to the surrounding layout and
is not baked into the SVG. The small PNG mark exports are centered on a
square transparent canvas; they still need surrounding clear space.

The horizontal artwork has a `582 × 128` viewBox. WebUI lettering is
approximately 57% of the HAMi letter height. At 32px artwork height, the
HAMi letters are 20.41px high, the WebUI letters are 11.68px high, and the
gap between the wordmarks is 7px. These proportions are already encoded
in the SVG; consumers should scale the complete artwork.

| Element | Light background | Dark background |
| --- | --- | --- |
| HAMi green | `#0FD05D` | `#0FD05D` |
| HAMi lettering | `#1B1C1D` | `#F3F4F5` |
| WebUI lettering | `#4A4D52` | `#C2C6CA` |

## PNG sizes

The checked-in PNGs use transparent backgrounds and are generated from the
SVGs without changing the geometry or palette.

| Variant | Export sizes | Filename pattern |
| --- | --- | --- |
| Horizontal, light and dark backgrounds | 291 × 64, 582 × 128, 1164 × 256 | `hami-webui-horizontal-{light,dark}-{64,128,256}h.png` |
| Melon mark, centered square canvas | 32, 64, 128, 256px square | `hami-mark-{32,64,128,256}.png` |

Use an export at least twice the displayed size on high-density screens;
use SVG when the required size is larger than an available export.
The 32px mark is an icon-sized rendition of the original artwork. The
application continues to use its existing favicon.

## Use in a README

From the repository root:

```html
<picture>
  <source media="(prefers-color-scheme: dark)"
    srcset="brand/svg/hami-webui-horizontal-dark.svg">
  <img src="brand/svg/hami-webui-horizontal-light.svg"
    alt="HAMi WebUI" width="320">
</picture>
```

## Application use and maintenance

The Vite `@brand` alias resolves directly to `brand/svg/`. The application
imports the light-background horizontal SVG from this directory, so the
sidebar and downloadable artwork share a single source. Docker includes
only the SVG masters in its build stage; documentation and PNG exports
are not needed by the running application.

Edit the SVG masters first, then regenerate the checked-in PNGs:

```sh
# Requires the optional rsvg-convert command from librsvg.
# macOS: brew install librsvg
# Debian/Ubuntu: apt-get install librsvg2-bin
sh brand/export-png.sh
```

The current exports were produced with `rsvg-convert 2.62.0`. No image
tool is required to run or build HAMi WebUI. When changing the artwork,
review both backgrounds, the 32px navigation size, and the PNG exports;
keep the SVGs free of embedded fonts, raster images, and external links.

## Origin and references

The melon and HAMi letter paths are preserved from the
[CNCF HAMi artwork](https://github.com/cncf/artwork/blob/main/projects/hami/horizontal/color/hami-horizontal-color-black.svg).
The source SVG used for this version has SHA-256
`63760e1a502516537e4a9e20bc2d8a49ed11ebfd33b1db8331c6ed624b92c872`.
Uniform scale and placement were adjusted to compose the horizontal logo.
The smaller WebUI outlines and this composition are maintained in this
repository; this package does not claim that the combined WebUI artwork
is an official CNCF artwork variant or an endorsement by CNCF.

This package follows the practical organization used by
[CNCF artwork](https://github.com/cncf/artwork) and the
[Kubernetes branding guide](https://github.com/kubernetes/kubernetes/blob/master/logo/usage_guidelines.md):
keep reusable artwork together, provide vector and raster formats, name
variants by their use, and document source and usage. The spacing and
size recommendations above apply to this WebUI composition.

HAMi trademark and logo use remains subject to the applicable
[CNCF artwork trademark policy](https://github.com/cncf/artwork/blob/main/LICENSE.md).
The repository's software license does not replace those trademark terms.
