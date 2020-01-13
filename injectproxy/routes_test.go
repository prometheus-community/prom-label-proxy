package injectproxy

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var okResponse = []byte(`ok`)

// checkQueryParameterHandler verifies that the request contains the given parameter key/value exactly once.
func checkQueryParameterHandler(key string, value string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		values, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			http.Error(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
		}
		// Verify that the client provides the parameter only once.
		if len(values[key]) != 1 {
			http.Error(w, fmt.Sprintf("expected 1 parameter %q, got %d", key, len(values[key])), http.StatusInternalServerError)
		}
		if values.Get(key) != value {
			http.Error(w, fmt.Sprintf("expected parameter %q with value %q, got %q", key, value, values.Get(key)), http.StatusInternalServerError)
		}
		w.Write(okResponse)
	})
}

// mockUpstream simulates an upstream HTTP server. It runs on localhost.
type mockUpstream struct {
	h   http.Handler
	srv *httptest.Server
	url *url.URL
}

func newMockUpstream(h http.Handler) *mockUpstream {
	m := mockUpstream{h: h}

	m.srv = httptest.NewServer(&m)

	u, err := url.Parse(m.srv.URL)
	if err != nil {
		panic(err)
	}
	m.url = u

	return &m
}

func (m *mockUpstream) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m.h.ServeHTTP(w, req)
}

func (m *mockUpstream) Close() {
	m.srv.Close()
}

const proxyLabel = "namespace"

func TestEndpointNotImplemented(t *testing.T) {
	m := newMockUpstream(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write(okResponse)
	}))
	defer m.Close()
	r := NewRoutes(m.url, proxyLabel)

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

		expCode  int
		expMatch string
		expBody  []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "match" parameter.
			labelv:   "default",
			expCode:  http.StatusOK,
			expMatch: `{namespace="default"}`,
			expBody:  okResponse,
		},
		{
			// Many "match" parameters.
			labelv:   "default",
			matches:  []string{`name={job="prometheus"}`, `{__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: `{namespace="default"}`,
			expBody:  okResponse,
		},
	} {
		t.Run(strings.Join(tc.matches, "&"), func(t *testing.T) {
			m := newMockUpstream(checkQueryParameterHandler("match[]", tc.expMatch))
			defer m.Close()
			r := NewRoutes(m.url, proxyLabel)

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
			body, _ := ioutil.ReadAll(resp.Body)
			defer resp.Body.Close()

			if resp.StatusCode != tc.expCode {
				t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
				t.Logf("%s", string(body))
				t.FailNow()
			}
			if resp.StatusCode != http.StatusOK {
				return
			}

			if string(body) != string(tc.expBody) {
				t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
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
			// Query without a vector selector.
			labelv:       "default",
			promQuery:    "up",
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a vector selector.
			labelv:       "default",
			promQuery:    `up{namespace="other"}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a vector selector.
			labelv:       "default",
			promQuery:    `up{namespace="default",}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a scalar.
			labelv:       "default",
			promQuery:    "1",
			expCode:      http.StatusOK,
			expPromQuery: `1`,
			expBody:      okResponse,
		},
		{
			// Query with a function.
			labelv:       "default",
			promQuery:    "time()",
			expCode:      http.StatusOK,
			expPromQuery: `time()`,
			expBody:      okResponse,
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
				m := newMockUpstream(checkQueryParameterHandler("query", tc.expPromQuery))
				defer m.Close()
				r := NewRoutes(m.url, proxyLabel)

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
				body, _ := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
				}
			})
		}
	}
}
