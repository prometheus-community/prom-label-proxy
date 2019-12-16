package injectproxy

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var mockResponse = []byte("ok")

type mockHandler struct {
	values url.Values
}

func (m *mockHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m.values, _ = url.ParseQuery(req.URL.RawQuery)
	w.Write(mockResponse)
}

const proxyLabel = "namespace"

func TestEndpointNotImplemented(t *testing.T) {
	r := NewRoutes(nil, proxyLabel)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://prometheus.example.com/graph", nil)

	r.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status code 404, got %d", resp.StatusCode)
	}
}

func TestFederate(t *testing.T) {
	for _, tc := range []struct {
		labelv  string
		matches []string

		expCode    int
		expMatches []string
		expBody    []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "match" parameter.
			labelv:     "default",
			expCode:    http.StatusOK,
			expMatches: []string{`{namespace="default"}`},
			expBody:    mockResponse,
		},
		{
			// "match" parameters.
			labelv:     "default",
			matches:    []string{`name={job="prometheus"}`, `{__name__=~"job:.*"}`},
			expCode:    http.StatusOK,
			expMatches: []string{`{namespace="default"}`},
			expBody:    mockResponse,
		},
	} {
		t.Run(strings.Join(tc.matches, "&"), func(t *testing.T) {
			h := &mockHandler{}
			r := NewRoutes(h, proxyLabel)

			u, err := url.Parse("http://prometheus.example.com/federate")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			for _, m := range tc.matches {
				q.Add("match[]", m)
			}
			q.Set(proxyLabel, tc.labelv)
			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", u.String(), nil)
			r.ServeHTTP(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expCode {
				t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
			}
			if resp.StatusCode != http.StatusOK {
				return
			}

			body, _ := ioutil.ReadAll(resp.Body)
			if string(body) != string(tc.expBody) {
				t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
			}

			matches := h.values["match[]"]
			if len(matches) != len(tc.expMatches) {
				t.Fatalf("expected %d matches, got %d", len(tc.expMatches), len(matches))
			}
			for i := range tc.expMatches {
				if matches[i] != tc.expMatches[i] {
					t.Fatalf("expected match %q, got %q", tc.expMatches[i], matches[i])
				}
			}
		})
	}
}

func TestQuery(t *testing.T) {
	for _, tc := range []struct {
		labelv    string
		promQuery string

		expCode      int
		expPromQuery string
		expBody      []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "query" parameter returns 200 with empty body.
			labelv:  "default",
			expCode: http.StatusOK,
		},
		{
			// Vector selector.
			labelv:       "default",
			promQuery:    "up",
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      mockResponse,
		},
		{
			// Scalar.
			labelv:       "default",
			promQuery:    "1",
			expCode:      http.StatusOK,
			expPromQuery: `1`,
			expBody:      mockResponse,
		},
		{
			// Function.
			labelv:       "default",
			promQuery:    "time()",
			expCode:      http.StatusOK,
			expPromQuery: `time()`,
			expBody:      mockResponse,
		},
		{
			// An invalid expression returns 200 with empty body.
			labelv:    "default",
			promQuery: "up +",
			expCode:   http.StatusOK,
		},
	} {
		for _, endpoint := range []string{"query", "query_range"} {
			t.Run(endpoint+"/"+tc.promQuery, func(t *testing.T) {
				h := &mockHandler{}
				r := NewRoutes(h, proxyLabel)

				u, err := url.Parse("http://prometheus.example.com/api/v1/" + endpoint)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				q.Set("query", tc.promQuery)
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", u.String(), nil)
				r.ServeHTTP(w, req)

				resp := w.Result()
				if resp.StatusCode != tc.expCode {
					t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				body, _ := ioutil.ReadAll(resp.Body)
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
				}

				if h.values.Get("query") != tc.expPromQuery {
					t.Fatalf("expected PromQL query %q, got %q", tc.expPromQuery, h.values.Get("query"))
				}
			})
		}
	}
}
