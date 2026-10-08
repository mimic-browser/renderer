# Maintained renderer branch

This branch starts from upstream `go-webengine/engine` v0.4.3 and keeps its BSD-3-Clause license and history. It contains only the packages imported by Mimic's static screenshot pipeline: `css`, `dom`, `layout`, `paint`, and `static`. JavaScript execution, module bundling, navigation, pagination, browser-like fetching, and their dependencies are removed.

The static path uses the same CSS, layout, and paint implementations as the earlier `DisableJS` path. A pixel regression test records representative outputs from that path, including external CSS, SVG, generated text, grid layout, and form controls. The upstream v0.5.1 rotated-content buffer fix is retained. Other local fixes improve grid shorthand, `@supports`, SVG rasterization, layout, controls, and literal CSS generated text.

`static.RenderHTML` remains an approximate painter. It does not define Mimic's DOM or layout semantics, execute page scripts, or produce a Chrome screenshot.
