// Copyright (c) the go-webengine/engine authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package static renders HTML without importing the engine's JavaScript runtime.
// It uses the same CSS, layout, image, font, and paint packages as engine.
package static

import (
	"bytes"
	"context"
	"image"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/go-webengine/engine/css"
	"github.com/go-webengine/engine/dom"
	"github.com/go-webengine/engine/layout"
	"github.com/go-webengine/engine/paint"
)

type Document struct {
	URL  string
	Root *dom.Node
}

type Engine struct {
	Client          *http.Client
	MaxImages       int
	MaxVectorImages int
	Backdrop        css.Color
}

func New() *Engine {
	return &Engine{Client: &http.Client{Timeout: 30 * time.Second}, MaxImages: 40, MaxVectorImages: 400}
}

type renderPass struct {
	sheets  []string
	sm      css.StyleMap
	box     *layout.Box
	height  float64
	imgSize map[*dom.Node][2]float64
	imgs    map[*dom.Node]image.Image
	bgImgs  map[string]image.Image
}

// RenderHTML paints an already available document. It does not execute scripts.
func (e *Engine) RenderHTML(ctx context.Context, htmlSrc, baseURL string, viewport image.Rectangle) (*image.RGBA, error) {
	root, err := dom.Parse(htmlSrc)
	if err != nil {
		return nil, err
	}
	doc := &Document{URL: baseURL, Root: root}
	fonts := paint.NewFonts()
	vpW, vpH := viewport.Dx(), viewport.Dy()
	if vpW <= 0 {
		vpW = 1024
	}
	if vpH <= 0 {
		vpH = 768
	}
	rp := &renderPass{}
	rp.sheets = e.loadStylesheets(ctx, doc, css.Media{Width: float64(vpW)})
	RegisterFontFaces(fonts, e.LoadFontFaces(ctx, doc, rp.sheets, css.Media{Width: float64(vpW)}))
	rp.sm = css.CascadeVW(root, float64(vpW), rp.sheets)
	rp.imgSize, rp.imgs = e.loadImages(ctx, doc, rp.sm, vpW)
	rp.bgImgs = e.loadBackgroundImages(ctx, doc, rp.sm)
	rp.box, rp.height = layout.LayoutDocumentViewport(root, rp.sm, float64(vpW), float64(vpH), fonts, rp.imgSize)
	rp.sm, rp.box, rp.height = layoutWithContainers(root, rp.sheets, float64(vpW), float64(vpH), fonts, rp.imgSize, rp.sm, rp.box, rp.height)
	css.AddGeneratedText(root, rp.sm, rp.sheets, css.Media{Width: float64(vpW)})
	rp.box, rp.height = layout.LayoutDocumentViewport(root, rp.sm, float64(vpW), float64(vpH), fonts, rp.imgSize)
	canvasH := max(viewport.Dy(), int(rp.height), 1)
	img := image.NewRGBA(image.Rect(0, 0, vpW, canvasH))
	if e.Backdrop.A > 0 {
		fillColor(img, e.Backdrop)
	} else {
		fillWhite(img)
	}
	if bg, ok := pageBackground(root, rp.sm); ok {
		fillColor(img, bg)
	}
	paint.PaintFull(img, rp.box, fonts, rp.imgs, rp.bgImgs)
	return img, nil
}

func (e *Engine) get(ctx context.Context, rawurl string) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return nil, "", "", err
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", "", err
	}
	return body, resp.Request.URL.String(), resp.Header.Get("Content-Type"), nil
}

func (e *Engine) fetchResource(ctx context.Context, rawURL string, limit int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false
	}
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return data, err == nil
}

func decodeCharset(body []byte, contentType string) ([]byte, error) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func fillWhite(img *image.RGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = 255
		img.Pix[i+1] = 255
		img.Pix[i+2] = 255
		img.Pix[i+3] = 255
	}
}

func fillColor(img *image.RGBA, c css.Color) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = 255
	}
}

// pageBackground returns the opaque backdrop colour to paint under the whole
// viewport: the body's background-color if set, else the html element's.
func pageBackground(root *dom.Node, sm css.StyleMap) (css.Color, bool) {
	for _, tag := range []string{"body", "html"} {
		if el := dom.Find(root, tag); el != nil {
			if st := sm[el]; st != nil && st.Background.A > 0 {
				return st.Background, true
			}
		}
	}
	return css.Color{}, false
}

// External-stylesheet fetch limits (safety: bounded work per page).
const (
	// maxExternalSheets was 20 until a live render of github.com/golang/go
	// showed exactly why that undercounts a real modern site: it ships 38
	// separate <link rel=stylesheet> tags (per-component CSS modules plus
	// several mutually-exclusive colour-scheme variants), and the ONE sheet
	// holding its header's hide/sr-only rules (visibility, clip-path,
	// position:fixed for its mega-menu dropdowns) happened to be 38th вЂ”
	// dropped past the old cap, so that markup rendered fully unstyled and
	// visible: the raw text of every dropdown menu, all concatenated at the
	// top of the page. The cap is not "how many sheets can a page have" (a
	// browser has no such limit) but "how much network/memory work is this
	// engine willing to do per render" вЂ” 64 comfortably covers a
	// component-styled site like this one while still bounding a
	// pathological page's fetch fan-out.
	maxExternalSheets    = 64
	maxSheetBytes        = 4 << 20 // 4 MB per sheet
	maxImportDepth       = 2       // @import nesting levels honoured
	externalSheetTimeout = 10 * time.Second
)

// fetchExternalSheets resolves and fetches every applicable
// <link rel="stylesheet"> for doc, plus their leading @import chains, returning
// the sheet texts in cascade order (imports before importer, links in document
// order). Only http/https absolute URLs are fetched; failures degrade
// gracefully (that sheet is skipped, the page still renders). vw is used to
// drop media-query-excluded links (e.g. print-only).
func (e *Engine) fetchExternalSheets(ctx context.Context, doc *Document, vw float64) []string {
	return e.loadStylesheets(ctx, doc, css.Media{Width: vw})
}

// loadStylesheets is fetchExternalSheets against a css.Media: under print,
// a <link media="print"> and a print-only @import are fetched and the
// screen-only ones skipped. Exported as LoadStylesheets.
func (e *Engine) loadStylesheets(ctx context.Context, doc *Document, m css.Media) []string {
	links := css.StylesheetLinks(doc.Root)
	if len(links) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, externalSheetTimeout)
	defer cancel()

	// Prefetch the applicable top-level sheet bodies CONCURRENTLY into a cache to
	// cut time-to-first-styled-paint (the initial progressive frame waits on this
	// fetch). This does NOT change the cascade: the sequential walk below still
	// applies sheets and their @imports in document order вЂ” it just reads bodies
	// the concurrent prefetch already retrieved. Deterministic: goroutines write
	// only their own result slot; the cache map is populated single-threaded.
	cache := map[string]sheetResult{}
	var pre []string
	seenPre := map[string]bool{}
	for _, ln := range links {
		if len(pre) >= maxExternalSheets {
			break
		}
		if !css.MediaAppliesTo(ln.Media, m) {
			continue
		}
		if abs, ok := resolveURL(doc.URL, ln.Href); ok && !seenPre[abs] {
			seenPre[abs] = true
			pre = append(pre, abs)
		}
	}
	e.prefetchSheets(ctx, pre, cache)

	seen := map[string]bool{}
	var out []string
	for _, ln := range links {
		if len(out) >= maxExternalSheets {
			break
		}
		if !css.MediaAppliesTo(ln.Media, m) {
			continue
		}
		abs, ok := resolveURL(doc.URL, ln.Href)
		if !ok {
			continue
		}
		out = e.appendSheet(ctx, abs, m, seen, out, 0, cache)
	}
	return out
}

// sheetResult is a fetched stylesheet body and whether the fetch succeeded.
type sheetResult struct {
	text string
	ok   bool
}

// prefetchSheets fetches urls concurrently, storing each result in cache keyed
// by URL. Each goroutine writes only its own slot, and the cache map is filled
// single-threaded after the fetches join, so the result is race-free and
// independent of completion order.
func (e *Engine) prefetchSheets(ctx context.Context, urls []string, cache map[string]sheetResult) {
	if len(urls) == 1 {
		t, ok := e.fetchSheetNet(ctx, urls[0])
		cache[urls[0]] = sheetResult{t, ok}
		return
	}
	res := make([]sheetResult, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			t, ok := e.fetchSheetNet(ctx, u)
			res[i] = sheetResult{t, ok}
		}(i, u)
	}
	wg.Wait()
	for i, u := range urls {
		cache[u] = res[i]
	}
}

// appendSheet fetches the sheet at absURL (if not already seen and within
// limits), recursively prepends its leading @import targets, then appends the
// sheet's own text. It returns the possibly-extended out slice.
func (e *Engine) appendSheet(ctx context.Context, absURL string, m css.Media, seen map[string]bool, out []string, depth int, cache map[string]sheetResult) []string {
	if depth > maxImportDepth || len(out) >= maxExternalSheets || seen[absURL] {
		return out
	}
	seen[absURL] = true
	text, ok := e.fetchSheet(ctx, absURL, cache)
	if !ok {
		return out
	}
	// @imports load (and thus cascade) before the importing sheet's own rules.
	imports, medias := css.ImportURLs(text)
	for i, imp := range imports {
		if !css.MediaAppliesTo(medias[i], m) {
			continue
		}
		if abs, ok := resolveURL(absURL, imp); ok {
			out = e.appendSheet(ctx, abs, m, seen, out, depth+1, cache)
		}
	}
	return append(out, text)
}

// fetchSheet returns a stylesheet body, preferring a value the concurrent
// prefetch already placed in cache and falling back to a direct network fetch
// (for @import targets, which are not prefetched).
func (e *Engine) fetchSheet(ctx context.Context, absURL string, cache map[string]sheetResult) (string, bool) {
	if r, ok := cache[absURL]; ok {
		return r.text, r.ok
	}
	return e.fetchSheetNet(ctx, absURL)
}

// fetchSheetNet performs the actual network fetch of a stylesheet body,
// bounded by maxSheetBytes. Non-2xx or oversized/undecodable responses report
// false.
func (e *Engine) fetchSheetNet(ctx context.Context, absURL string) (string, bool) {
	if !strings.HasPrefix(absURL, "http://") && !strings.HasPrefix(absURL, "https://") {
		return "", false
	}
	body, _, ctype, err := e.get(ctx, absURL)
	if err != nil {
		return "", false
	}
	if len(body) > maxSheetBytes {
		body = body[:maxSheetBytes]
	}
	utf8, err := decodeCharset(body, ctype)
	if err != nil {
		utf8 = body
	}
	return string(utf8), true
}

// resolveURL resolves ref against the document base URL.
func resolveURL(base, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	b, err := url.Parse(base)
	if err != nil {
		return "", false
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	return b.ResolveReference(r).String(), true
}
