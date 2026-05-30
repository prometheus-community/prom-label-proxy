// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package injectproxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func prometheusAPIError(w http.ResponseWriter, req *http.Request, errorMessage string, code int) {
	w.Header().Set("X-Proxy-Error-Logged", "true")

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)

	slog.Debug("API error returned to client",
		"status", code,
		"message", errorMessage,
		"path", req.URL.Path,
		"method", req.Method,
	)

	res := map[string]string{"status": "error", "errorType": "prom-label-proxy", "error": errorMessage}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.Error("Failed to encode json", "error", err)
	}
}
