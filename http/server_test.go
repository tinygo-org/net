// TINYGO: Tests for the custom ServeMux routing logic.
//
// These cover the three behaviours that previously diverged from the standard
// library's net/http.ServeMux (see issue #55):
//
//  1. {name...} multi-segment trailing wildcards,
//  2. the {$} end-of-path anchor, and
//  3. method-prefixed patterns ("GET /path").
//
// Plus precedence and no-regression cases for exact / {id} / subtree / host
// patterns. Expected outcomes mirror net/http.ServeMux on Go 1.26.
//
// NOTE: this package has no module-root go.mod (it imports TinyGo internals),
// so these tests are not run on the host; they document and lock the intended
// behaviour and run under TinyGo's test harness. They are written to use only
// the package's own exported/unexported API and a tiny in-test ResponseWriter
// so there is no dependency on net/http/httptest (which would import this
// package and create a cycle).

package http

import (
	"net/url"
	"testing"
)

// testResponseWriter is a minimal ResponseWriter; the routing tests never
// inspect the response body, they only need ServeHTTP to be callable so that
// path values get populated on the request.
type testResponseWriter struct {
	header Header
	code   int
}

func newTestResponseWriter() *testResponseWriter {
	return &testResponseWriter{header: make(Header)}
}

func (w *testResponseWriter) Header() Header              { return w.header }
func (w *testResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *testResponseWriter) WriteHeader(code int)        { w.code = code }

// newTestRequest builds a server-side *Request with just the fields the mux
// reads: Method, Host and URL.Path. RequestURI is set so ServeHTTP's "*" guard
// is not triggered.
func newTestRequest(method, host, path string) *Request {
	if method == "" {
		method = MethodGet
	}
	if host == "" {
		host = "example.com"
	}
	return &Request{
		Method:     method,
		Host:       host,
		URL:        &url.URL{Path: path},
		RequestURI: path,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

// route registers patterns on a fresh mux, dispatches a request and returns the
// matched pattern ("" means 404) together with any path values the matched
// pattern exposed via r.PathValue.
func route(t *testing.T, patterns []string, method, host, path string, valNames []string) (pattern string, vals map[string]string) {
	t.Helper()
	mux := NewServeMux()
	for _, p := range patterns {
		mux.HandleFunc(p, func(ResponseWriter, *Request) {})
	}

	r := newTestRequest(method, host, path)
	_, pattern = mux.Handler(r)

	vals = map[string]string{}
	if len(valNames) > 0 {
		// Drive the full ServeHTTP path so SetPathValue runs, then read back.
		captured := newTestRequest(method, host, path)
		valMux := NewServeMux()
		for _, p := range patterns {
			valMux.HandleFunc(p, func(_ ResponseWriter, req *Request) {
				captured = req
			})
		}
		valMux.ServeHTTP(newTestResponseWriter(), captured)
		for _, n := range valNames {
			vals[n] = captured.PathValue(n)
		}
	}
	return pattern, vals
}

func TestServeMux_Routing(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		method   string
		host     string
		path     string
		// wantPattern is the registered pattern expected to match; "" => 404.
		wantPattern string
		// wantVals maps a wildcard name to its expected captured value.
		wantVals map[string]string
	}{
		// --- exact match (no regression) ---
		{name: "exact-hit", patterns: []string{"/foo"}, path: "/foo", wantPattern: "/foo"},
		{name: "exact-miss", patterns: []string{"/foo"}, path: "/bar", wantPattern: ""},
		{name: "root-subtree-catches-all", patterns: []string{"/"}, path: "/anything", wantPattern: "/"},

		// --- single {id} (no regression) ---
		{name: "id-hit", patterns: []string{"/users/{id}"}, path: "/users/42",
			wantPattern: "/users/{id}", wantVals: map[string]string{"id": "42"}},
		{name: "id-too-many-segments-404", patterns: []string{"/users/{id}"}, path: "/users/42/x", wantPattern: ""},
		{name: "id-two", patterns: []string{"/users/{id}/orders/{oid}"}, path: "/users/7/orders/9",
			wantPattern: "/users/{id}/orders/{oid}", wantVals: map[string]string{"id": "7", "oid": "9"}},
		{name: "id-empty-segment-404", patterns: []string{"/users/{id}"}, path: "/users/", wantPattern: ""},

		// --- trailing-slash subtree (no regression) ---
		{name: "subtree-hit", patterns: []string{"/static/"}, path: "/static/a/b", wantPattern: "/static/"},
		{name: "subtree-exact-prefix", patterns: []string{"/static/"}, path: "/static/", wantPattern: "/static/"},

		// --- Bug 1: {name...} multi-segment wildcard ---
		{name: "multi-many", patterns: []string{"/files/{path...}"}, path: "/files/a/b/c",
			wantPattern: "/files/{path...}", wantVals: map[string]string{"path": "a/b/c"}},
		{name: "multi-one", patterns: []string{"/files/{path...}"}, path: "/files/a",
			wantPattern: "/files/{path...}", wantVals: map[string]string{"path": "a"}},
		{name: "multi-empty", patterns: []string{"/files/{path...}"}, path: "/files/",
			wantPattern: "/files/{path...}", wantVals: map[string]string{"path": ""}},
		{name: "multi-wrong-prefix-404", patterns: []string{"/files/{path...}"}, path: "/other/x", wantPattern: ""},

		// --- Bug 1 precedence: literal > {id} > {x...} at same end-path ---
		{name: "multi-vs-literal-literal-wins", patterns: []string{"/files/{path...}", "/files/special"},
			path: "/files/special", wantPattern: "/files/special"},
		{name: "id-beats-multi-on-one-seg", patterns: []string{"/a/{id}", "/a/{x...}"}, path: "/a/one",
			wantPattern: "/a/{id}", wantVals: map[string]string{"id": "one"}},
		{name: "multi-wins-on-two-seg", patterns: []string{"/a/{id}", "/a/{x...}"}, path: "/a/one/two",
			wantPattern: "/a/{x...}", wantVals: map[string]string{"x": "one/two"}},

		// --- Bug 2: {$} end-of-path anchor ---
		{name: "anchor-hit", patterns: []string{"/exact/{$}"}, path: "/exact/", wantPattern: "/exact/{$}"},
		{name: "anchor-reject-sub-404", patterns: []string{"/exact/{$}"}, path: "/exact/sub", wantPattern: ""},
		{name: "anchor-no-bogus-value", patterns: []string{"/exact/{$}"}, path: "/exact/",
			wantPattern: "/exact/{$}", wantVals: map[string]string{"$": ""}},
		{name: "root-anchor-hit", patterns: []string{"/{$}"}, path: "/", wantPattern: "/{$}"},
		{name: "root-anchor-reject-404", patterns: []string{"/{$}"}, path: "/x", wantPattern: ""},

		// --- Bug 2 precedence: {$} beats both subtree and {x...} at "/d/" ---
		{name: "anchor-beats-subtree-at-end", patterns: []string{"/d/{$}", "/d/"}, path: "/d/", wantPattern: "/d/{$}"},
		{name: "subtree-still-catches-deep", patterns: []string{"/d/{$}", "/d/"}, path: "/d/deep", wantPattern: "/d/"},
		{name: "anchor-beats-multi-at-end", patterns: []string{"/d/{$}", "/d/{x...}"}, path: "/d/", wantPattern: "/d/{$}"},
		{name: "multi-catches-deep-over-anchor", patterns: []string{"/d/{$}", "/d/{x...}"}, path: "/d/z",
			wantPattern: "/d/{x...}", wantVals: map[string]string{"x": "z"}},

		// --- Bug 3: method-prefixed patterns ---
		{name: "method-get-hit", patterns: []string{"GET /m"}, method: "GET", path: "/m", wantPattern: "GET /m"},
		{name: "method-get-on-post-404", patterns: []string{"GET /m"}, method: "POST", path: "/m", wantPattern: ""},
		{name: "method-pick-post", patterns: []string{"GET /m", "POST /m"}, method: "POST", path: "/m", wantPattern: "POST /m"},
		{name: "method-and-plain-falls-through", patterns: []string{"GET /m", "/m"}, method: "DELETE", path: "/m", wantPattern: "/m"},
		{name: "method-id", patterns: []string{"GET /u/{id}"}, method: "GET", path: "/u/5",
			wantPattern: "GET /u/{id}", wantVals: map[string]string{"id": "5"}},
		{name: "method-multi", patterns: []string{"POST /up/{rest...}"}, method: "POST", path: "/up/a/b",
			wantPattern: "POST /up/{rest...}", wantVals: map[string]string{"rest": "a/b"}},

		// --- Bug 3: a method prefix must NOT enable host routing and corrupt
		// other plain routes (the original bug set mux.hosts=true). ---
		{name: "method-no-hosts-corruption-plain", patterns: []string{"GET /a", "/p"}, method: "GET", path: "/p", wantPattern: "/p"},
		{name: "method-no-hosts-corruption-method", patterns: []string{"GET /a", "/p"}, method: "GET", path: "/a", wantPattern: "GET /a"},

		// --- method + specificity interaction ---
		{name: "methodless-multi-wins-on-method-miss", patterns: []string{"GET /a/{id}", "/a/{x...}"},
			method: "POST", path: "/a/one", wantPattern: "/a/{x...}", wantVals: map[string]string{"x": "one"}},
		{name: "method-id-wins-on-method-hit", patterns: []string{"GET /a/{id}", "/a/{x...}"},
			method: "GET", path: "/a/one", wantPattern: "GET /a/{id}", wantVals: map[string]string{"id": "one"}},

		// --- host patterns (no regression) ---
		{name: "host-pattern-match", patterns: []string{"example.com/h", "/h"}, host: "example.com", path: "/h",
			wantPattern: "example.com/h"},
		{name: "host-pattern-fallthrough", patterns: []string{"example.com/h", "/h"}, host: "other.com", path: "/h",
			wantPattern: "/h"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var valNames []string
			for n := range c.wantVals {
				valNames = append(valNames, n)
			}

			gotPattern, gotVals := route(t, c.patterns, c.method, c.host, c.path, valNames)

			if gotPattern != c.wantPattern {
				t.Fatalf("pattern: got %q, want %q", gotPattern, c.wantPattern)
			}
			for n, want := range c.wantVals {
				if got := gotVals[n]; got != want {
					t.Errorf("PathValue(%q): got %q, want %q", n, got, want)
				}
			}
		})
	}
}

// TestServeMux_MethodPrefixDoesNotSetHosts asserts the specific corruption from
// bug 3: registering a method-prefixed pattern must not flip mux.hosts, which
// would otherwise force every lookup through host+path first.
func TestServeMux_MethodPrefixDoesNotSetHosts(t *testing.T) {
	mux := NewServeMux()
	mux.HandleFunc("GET /foo", func(ResponseWriter, *Request) {})
	if mux.hosts {
		t.Fatalf("registering %q wrongly set mux.hosts = true", "GET /foo")
	}

	// A genuine host pattern still sets it.
	mux2 := NewServeMux()
	mux2.HandleFunc("example.com/foo", func(ResponseWriter, *Request) {})
	if !mux2.hosts {
		t.Fatalf("registering a host pattern should set mux.hosts = true")
	}
}

// TestServeMux_InvalidMethodOnlyPatternPanics asserts that a pattern that
// strips to an empty path (e.g. "GET ") is rejected, matching net/http.
func TestServeMux_InvalidMethodOnlyPatternPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Handle(%q) should panic on an empty path part", "GET ")
		}
	}()
	mux := NewServeMux()
	mux.HandleFunc("GET ", func(ResponseWriter, *Request) {})
}

// TestParseMethod checks the leading-method tokenizer used by Handle.
func TestParseMethod(t *testing.T) {
	cases := []struct {
		in         string
		wantMethod string
		wantPath   string
	}{
		{"/foo", "", "/foo"},
		{"GET /foo", "GET", "/foo"},
		{"POST /a/{id}", "POST", "/a/{id}"},
		{"example.com/foo", "", "example.com/foo"}, // host, not a method
		{"GET  /foo", "GET", "/foo"},               // extra spaces trimmed
		{"get /foo", "", "get /foo"},               // lower-case is not a method token
		{"GET", "", "GET"},                         // no space, treated as path
		{"GET /a b", "GET", "/a b"},                // only the first space splits
	}
	for _, c := range cases {
		gotMethod, gotPath := parseMethod(c.in)
		if gotMethod != c.wantMethod || gotPath != c.wantPath {
			t.Errorf("parseMethod(%q) = (%q, %q), want (%q, %q)",
				c.in, gotMethod, gotPath, c.wantMethod, c.wantPath)
		}
	}
}
