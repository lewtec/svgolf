# svgolf

Turn a PNG, especially a model-generated logo, into a simple editable SVG.

v1 is the trusted render pipeline: an SVG-like tree, an in-process renderer that must match **resvg** on the locked fixture set, a Cobra CLI, Search (Dumb adapter), and exact-match fuzz. v1 Search is one shot (Dumb): one epoch, then the iterator stops. Import `search/prelude` so adapters Register. Search owns Cost and rank.

Contract: [SPEC.md](SPEC.md).

## Tools

mise pins Go and resvg:

```
mise install
mise run test
mise run build
mise run fuzz
```

`mise run test` puts resvg 0.47.0 on PATH. Match is exact RGBA vs resvg.

Build the CLI without mise when Go is already installed:

```
go build -o bin/svgolf ./cmd/svgolf
```

resvg is the verify oracle. `render` and `vectorize` do not call it. `verify`, `preview`, fuzz, and tests that compare to resvg do.

## CLI

```
svgolf render    in.svg -o out.png
svgolf verify    in.svg [--diff path]
svgolf vectorize in.png -o out.svg [--search NAME] [--search-opt key=value]
svgolf preview   --search NAME [--search-opt key=value]
```

`verify` exits 0 only when every pixel matches resvg (and Encode does not drift). On a pixel mismatch it writes `<input>.diff.png`.

`vectorize` is PNG → `FromImage` → `search.New` → epochs → Encode `Last`. Native size. Only a Search adapter may scale. `--epochs DIR` writes each epoch as `NNN.svg` + `NNN.png` and overwrites `last.svg` + `last.png`. `--search-opt key=value` repeats. The CLI forwards the pairs. `stack` accepts `epochs` (0 is no cap), `beam`, `command-cost`, `minimum-island`, and `minimum-error`. Unset keys keep the built-in search. `stack` may emit an open stroked path when that stroke paints the island and nothing else. Register with `search.Register` in the adapter `init`; blank-import `search/prelude`. `preview` runs that Search on `testdata/eval` into `testdata/preview` (want + SVG + resvg at intrinsic size). Ships `dumb` and `stack`.

## Layout

| Path | Role |
| --- | --- |
| `pkg/svg` | tree, `New*`, Encode, Parse |
| `pkg/render` | in-process → `image.NRGBA` |
| `cmd/svgolf` | Cobra |
| `internal/` | resvg oracle, Search (`dumb` + prelude), Loss (Pixels/RMSE), verify |
| `testdata/eval` | full-size scenes: bliss + LEWTEC logos |

## Not in v1

Looping Search, other Loss formulas, transform, clip, mask, radial/mesh gradients, `svgolf fuzz` (use `go test -fuzz=FuzzRender`). 2-stop linear gradients are in.
