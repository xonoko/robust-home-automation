package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func serve(t *testing.T, h http.Handler, p string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, nil))
	return rec
}

func TestSPAFallback(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("js")},
	})

	if rec := serve(t, h, "/assets/app.js"); rec.Body.String() != "js" {
		t.Errorf("static file: got %q", rec.Body.String())
	}
	if rec := serve(t, h, "/rooms/kitchen"); rec.Code != http.StatusOK || rec.Body.String() != "<html>app</html>" {
		t.Errorf("client route: got %d %q", rec.Code, rec.Body.String())
	}
	if rec := serve(t, h, "/api/v1/missing"); rec.Code != http.StatusNotFound {
		t.Errorf("api path: got %d", rec.Code)
	}
}

func TestNotBuilt(t *testing.T) {
	if rec := serve(t, handler(fstest.MapFS{}), "/"); rec.Code != http.StatusNotFound {
		t.Errorf("got %d", rec.Code)
	}
}
