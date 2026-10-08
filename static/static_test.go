package static_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-webengine/engine/static"
)

func TestRenderHTMLPreservesDisabledJSOutput(t *testing.T) {
	assets := map[string]string{
		"/style.css": `body { margin: 8px; background: #f9f9f9; } .row { display: grid; grid-template-columns: 100px 1fr; gap: 8px; } label::before { content: "Choose "; }`,
		"/icon.svg":  `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><path fill="blue" d="M0 0h24v24H0z"/></svg>`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := assets[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	cases := []struct{ html, pixelSHA256 string }{
		{`<html><head><link rel="stylesheet" href="/style.css"></head><body><div class="row"><img src="/icon.svg"><label>Name and Scope</label></div></body></html>`, "3a598eb63f2d861c7a2e806701c4eddd054ff77bacfc04e386ca0a0d766fed70"},
		{`<html><body><h1>Title</h1><input type="radio" checked><span>Standard</span><div style="position:absolute;top:80px;background:red;width:20px;height:20px"></div></body></html>`, "51503e34bc9555d0cdadd68f2e62acb5db5115e8f0393617ae1c70feda7f6e7b"},
	}
	for _, test := range cases {
		next := static.New()
		next.Client = server.Client()
		got, err := next.RenderHTML(context.Background(), test.html, server.URL+"/", image.Rect(0, 0, 320, 180))
		if err != nil {
			t.Fatal(err)
		}
		if gotSHA := fmt.Sprintf("%x", sha256.Sum256(got.Pix)); gotSHA != test.pixelSHA256 {
			t.Fatalf("static render changed for %q: pixel SHA256 %s, want %s", test.html, gotSHA, test.pixelSHA256)
		}
	}
}
