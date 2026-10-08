# Mimic renderer

This repository is a [fork of go-webengine/engine](https://github.com/go-webengine/engine), based on v0.4.3. It retains the upstream Git history and BSD-3-Clause license. The `mimic-go1.26` branch contains the CSS, DOM, layout, image, font, SVG, and paint packages needed to produce approximate screenshots for [Mimic](https://github.com/mimic-browser/runtime) on Go 1.26.4.

The `static` package renders supplied HTML with a caller-provided HTTP client. It does not execute scripts, navigate pages, or load resources beyond what the client permits. Mimic supplies an immutable snapshot and a transport that serves only snapshot assets. The renderer's CSS and layout are independent of Mimic's script-visible state; visual accuracy is not guaranteed.

```go
renderer := static.New()
renderer.Client = client // Restrict access to the supplied snapshot resources.
img, err := renderer.RenderHTML(ctx, html, baseURL, image.Rect(0, 0, width, height))
```

The module path remains `github.com/go-webengine/engine` for imports of the retained packages. Mimic pins this fork with a Go module `replace` directive. The branch includes the relevant rotated-content bounds fix from upstream v0.5.1 and renderer fixes documented in [MIMIC.md](MIMIC.md).
