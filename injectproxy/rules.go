package injectproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
)

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
}

func getAPIResponse(resp *http.Response) (*apiResponse, error) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apir apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apir); err != nil {
		return nil, err
	}

	if apir.Status != "success" {
		return nil, fmt.Errorf("unexpected response status: %q", apir.Status)
	}

	return &apir, nil
}

func (a *apiResponse) setData(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.Data = json.RawMessage(b)
	return nil
}

type rulesData struct {
	RuleGroups []*ruleGroup `json:"groups"`
}

type ruleGroup struct {
	Name     string  `json:"name"`
	File     string  `json:"file"`
	Rules    []rule  `json:"rules"`
	Interval float64 `json:"interval"`
}

type rule struct {
	*alertingRule
	*recordingRule
}

func (r *rule) Labels() labels.Labels {
	if r.alertingRule != nil {
		return r.alertingRule.Labels
	}
	return r.recordingRule.Labels
}

// MarshalJSON implements the json.Marshaler interface for rule.
func (r *rule) MarshalJSON() ([]byte, error) {
	if r.alertingRule != nil {
		return json.Marshal(r.alertingRule)
	}
	return json.Marshal(r.recordingRule)
}

// UnmarshalJSON implements the json.Unmarshaler interface for rule.
func (r *rule) UnmarshalJSON(b []byte) error {
	var ruleType struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &ruleType); err != nil {
		return err
	}
	switch ruleType.Type {
	case "alerting":
		var alertingr alertingRule
		if err := json.Unmarshal(b, &alertingr); err != nil {
			return err
		}
		r.alertingRule = &alertingr
	case "recording":
		var recordingr recordingRule
		if err := json.Unmarshal(b, &recordingr); err != nil {
			return err
		}
		r.recordingRule = &recordingr
	default:
		return fmt.Errorf("failed to unmarshal rule: unknown type %q", ruleType.Type)
	}

	return nil
}

type alertingRule struct {
	Name        string        `json:"name"`
	Query       string        `json:"query"`
	Duration    float64       `json:"duration"`
	Labels      labels.Labels `json:"labels"`
	Annotations labels.Labels `json:"annotations"`
	Alerts      []*alert      `json:"alerts"`
	Health      string        `json:"health"`
	LastError   string        `json:"lastError,omitempty"`
	// Type of an alertingRule is always "alerting".
	Type string `json:"type"`
}

type recordingRule struct {
	Name      string        `json:"name"`
	Query     string        `json:"query"`
	Labels    labels.Labels `json:"labels,omitempty"`
	Health    string        `json:"health"`
	LastError string        `json:"lastError,omitempty"`
	// Type of a recordingRule is always "recording".
	Type string `json:"type"`
}

type alertsData struct {
	Alerts []*alert `json:"alerts"`
}

type alert struct {
	Labels      labels.Labels `json:"labels"`
	Annotations labels.Labels `json:"annotations"`
	State       string        `json:"state"`
	ActiveAt    *time.Time    `json:"activeAt,omitempty"`
	Value       string        `json:"value"`
}

// modifyAPIResponse unwraps the Prometheus API response, passes the data field
// to the process function and replaces the result in the response.
func modifyAPIResponse(resp *http.Response, process func([]byte) (interface{}, error)) error {
	if resp.StatusCode != http.StatusOK {
		// Pass non-200 responses as-is.
		return nil
	}

	apir, err := getAPIResponse(resp)
	if err != nil {
		return errors.Wrap(err, "can't decode API response")
	}

	v, err := process([]byte(apir.Data))
	if err != nil {
		return err
	}

	if err := apir.setData(v); err != nil {
		return errors.Wrap(err, "can't set data")
	}

	var buf bytes.Buffer
	if err = json.NewEncoder(&buf).Encode(apir); err != nil {
		return errors.Wrap(err, "can't encode API response")
	}
	resp.Body = ioutil.NopCloser(&buf)
	resp.Header["Content-Length"] = []string{fmt.Sprint(buf.Len())}

	return nil
}

func (r *routes) rules(resp *http.Response) error {
	return modifyAPIResponse(resp, func(b []byte) (interface{}, error) {
		var rgs rulesData
		if err := json.Unmarshal([]byte(b), &rgs); err != nil {
			return nil, errors.Wrap(err, "can't decode rules data")
		}

		lvalue := mustLabelValue(resp.Request.Context())
		filtered := []*ruleGroup{}
		for _, rg := range rgs.RuleGroups {
			var rules []rule
			for _, rule := range rg.Rules {
				for _, lbl := range rule.Labels() {
					if lbl.Name == r.label && lbl.Value == lvalue {
						rules = append(rules, rule)
						break
					}
				}
			}
			if len(rules) > 0 {
				rg.Rules = rules
				filtered = append(filtered, rg)
			}
		}

		return &rulesData{RuleGroups: filtered}, nil
	})
}

func (r *routes) alerts(resp *http.Response) error {
	return modifyAPIResponse(resp, func(b []byte) (interface{}, error) {
		var data alertsData
		if err := json.Unmarshal([]byte(b), &data); err != nil {
			return nil, errors.Wrap(err, "can't decode alerts data")
		}

		lvalue := mustLabelValue(resp.Request.Context())
		filtered := []*alert{}
		for _, alert := range data.Alerts {
			for _, lbl := range alert.Labels {
				if lbl.Name == r.label && lbl.Value == lvalue {
					filtered = append(filtered, alert)
					break
				}
			}
		}

		return &alertsData{Alerts: filtered}, nil
	})
}
