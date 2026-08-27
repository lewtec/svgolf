# svgolf

Turn a PNG, especially a model-generated logo, into a simple editable SVG.

v1 is the trusted render pipeline: an SVG-like tree, an in-process renderer that must match **resvg** on the locked fixture set, a Cobra CLI, a dumb generator, and exact-match fuzz. v1 does not search.

Contract: [SPEC.md](SPEC.md).

## Tools

All tools come from mise. Do not install Go or resvg on the host.

```
mise install
mise run test
mise run build
mise run fuzz
```

`mise run test` is the only supported test entry. It puts resvg 0.47.0 on PATH.

Match is exact RGBA vs resvg. No slop.

## CLI

```
svgolf render    in.svg -o out.png
svgolf verify    in.svg [--diff path]
svgolf vectorize in.png -o out.svg [--colors N]
```

`verify` exits 0 only when every pixel matches resvg (and Encode does not drift). On a pixel mismatch it writes `<input>.diff.png`.

`vectorize` writes a stub: one rect per palette color, most-used first, concentric 75% shrink.

## Layout

| Path | Role |
| --- | --- |
| `pkg/svg` | tree, `New*`, Encode, Parse |
| `pkg/render` | in-process → `image.NRGBA` |
| `cmd/svgolf` | Cobra |
| `internal/` | resvg oracle, palette, dumb generator, verify |

## Not in v1

Search, Loss formula, path/Bézier primitives, transform, clip, mask, gradients, `svgolf fuzz` (use `go test -fuzz=FuzzRender`).
