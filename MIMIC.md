# Mimic renderer fork

This repository is a GitHub fork of [go-webengine/engine](https://github.com/go-webengine/engine). It retains the upstream BSD-3-Clause license and Git history. The `mimic-go1.26` branch starts from upstream v0.4.3 and is maintained for Mimic's approximate CDP screenshots on Go 1.26.4. The Go module path remains `github.com/go-webengine/engine` so existing imports can use a module replacement.

The branch includes the rotated-content buffer fix from upstream v0.5.1 and the `klauspost/compress` v1.18.7 dependency update from v0.5.0. It also fixes grid shorthand and `@supports` parsing, SVG rasterization, absolute and inline layout, font-size keywords, styled form controls, and literal CSS generated text used by Mimic's screenshot adapter.

The renderer produces approximate images. Its output is neither a Chrome screenshot nor Mimic's script-visible layout state.
