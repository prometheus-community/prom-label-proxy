package injectproxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
)

type routes struct {
	handler    http.Handler
	label      string
	forwarders map[string]func(http.ResponseWriter, *http.Request)
	modifiers  map[string]func(*http.Response) error
}

func NewRoutes(upstream *url.URL, label string) *routes {
	proxy := httputil.NewSingleHostReverseProxy(upstream)

	r := &routes{
		handler: proxy,
		label:   label,
	}
	r.forwarders = map[string]func(http.ResponseWriter, *http.Request){
		"/federate":           r.federate,
		"/api/v1/query":       r.query,
		"/api/v1/query_range": r.query,
		"/api/v1/alerts":      r.noop,
		"/api/v1/rules":       r.noop,
	}
	r.modifiers = map[string]func(*http.Response) error{
		"/api/v1/rules":  r.rules,
		"/api/v1/alerts": r.alerts,
	}
	proxy.ModifyResponse = r.ModifyResponse
	return r
}

func (r *routes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h, found := r.forwarders[req.URL.Path]
	if !found {
		http.NotFound(w, req)
		return
	}

	lvalue := req.URL.Query().Get(r.label)
	if lvalue == "" {
		http.Error(w, fmt.Sprintf("Bad request. The %q query parameter must be provided.", r.label), http.StatusBadRequest)
		return
	}
	req = req.WithContext(withLabelValue(req.Context(), lvalue))
	req.URL.Query().Del(r.label)

	h(w, req)
}

func (r *routes) ModifyResponse(resp *http.Response) error {
	m, found := r.modifiers[resp.Request.URL.Path]
	if !found {
		// Return the server's response unmodified.
		return nil
	}
	return m(resp)
}

type ctxKey int

const keyLabel ctxKey = iota

func mustLabelValue(ctx context.Context) string {
	label, ok := ctx.Value(keyLabel).(string)
	if !ok {
		panic(fmt.Sprintf("can't find the %q value in the context", keyLabel))
	}
	if label == "" {
		panic(fmt.Sprintf("empty %q value in the context", keyLabel))
	}
	return label
}

func withLabelValue(ctx context.Context, label string) context.Context {
	return context.WithValue(ctx, keyLabel, label)
}

func (r *routes) noop(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

func (r *routes) query(w http.ResponseWriter, req *http.Request) {
	expr, err := promql.ParseExpr(req.FormValue("query"))
	if err != nil {
		return
	}

	err = SetRecursive(expr, []*labels.Matcher{
		{
			Name:  r.label,
			Type:  labels.MatchEqual,
			Value: mustLabelValue(req.Context()),
		},
	})
	if err != nil {
		return
	}

	q := req.URL.Query()
	q.Set("query", expr.String())
	req.URL.RawQuery = q.Encode()

	r.handler.ServeHTTP(w, req)
}

func (r *routes) federate(w http.ResponseWriter, req *http.Request) {
	matcher := &labels.Matcher{
		Name:  r.label,
		Type:  labels.MatchEqual,
		Value: mustLabelValue(req.Context()),
	}

	q := req.URL.Query()
	q.Set("match[]", "{"+matcher.String()+"}")
	req.URL.RawQuery = q.Encode()

	r.handler.ServeHTTP(w, req)
}
