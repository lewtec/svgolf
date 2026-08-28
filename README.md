# svgolf

Turn a PNG, especially a model-generated logo, into a simple editable SVG.

v1 is the trusted render pipeline: an SVG-like tree, an in-process renderer that must match **resvg** on the locked fixture set, a Cobra CLI, Search (Dumb adapter), and exact-match fuzz. v1 Search is one shot (Dumb): one epoch, then the iterator stops. Import `search/prelude` so adapters Register. Search owns Cost and rank.

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
svgolf vectorize in.png -o out.svg [--search NAME]
svgolf preview   --search NAME
```

`verify` exits 0 only when every pixel matches resvg (and Encode does not drift). On a pixel mismatch it writes `<input>.diff.png`.

`vectorize` is PNG → `FromImage` → `search.New` → epochs → Encode `Last`. Native size. Only a Search adapter may scale. `--epochs DIR` writes each epoch as `NNN.svg` + `NNN.png` and overwrites `last.svg` + `last.png`. Register with `search.Register` in the adapter `init`; blank-import `search/prelude`. `preview` runs that Search on `testdata/eval` into `testdata/preview` (want + SVG + resvg at intrinsic size). Ships `dumb` and `stack`.

## Layout

| Path | Role |
| --- | --- |
| `pkg/svg` | tree, `New*`, Encode, Parse |
| `pkg/render` | in-process → `image.NRGBA` |
| `cmd/svgolf` | Cobra |
| `internal/` | resvg oracle, Search (`dumb` + prelude), Loss (Pixels/RMSE), verify |
| `testdata/eval` | full-size scenes: bliss + LEWTEC logos |

## Not in v1

Looping Search, other Loss formulas, transform, clip, mask, gradients, `svgolf fuzz` (use `go test -fuzz=FuzzRender`).
