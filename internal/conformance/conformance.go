// Package conformance calls every IR endpoint on the mock server and records
// which responses are successful, JSON, and labeled with the declared content type.
package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/atharv1730/SpecForge/internal/ir"
	"github.com/atharv1730/SpecForge/internal/mock"
)

// Report is the result of calling every endpoint once.
type Report struct {
	Passed   int
	Failed   int
	Failures []FailureDetail
}

// FailureDetail is one endpoint the mock server did not answer as the IR describes.
type FailureDetail struct {
	Method      string
	Path        string
	OperationID string
	Message     string
}

// Run starts the mock server on mockPort and calls every endpoint with example data
// through net/http. clientDir is the directory of the generated client.
func Run(api *ir.API, clientDir string, mockPort int) (*Report, error) {
	if api == nil {
		return nil, fmt.Errorf("conformance: nil API")
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", mockPort))
	if err != nil {
		return nil, fmt.Errorf("conformance: listen: %w", err)
	}
	srv := mock.NewServer(api, mockPort)
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + ln.Addr().String()
	report := &Report{}
	for _, ep := range api.Endpoints {
		if err := checkEndpoint(client, base, ep); err != nil {
			report.Failed++
			report.Failures = append(report.Failures, FailureDetail{
				Method:      ep.Method,
				Path:        ep.Path,
				OperationID: ep.OperationID,
				Message:     err.Error(),
			})
			continue
		}
		report.Passed++
	}
	return report, nil
}

func checkEndpoint(client *http.Client, base string, ep ir.Endpoint) error {
	req, err := newRequest(base, ep)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return validateResponse(ep, res, body)
}

func newRequest(base string, ep ir.Endpoint) (*http.Request, error) {
	rawURL, header, err := endpointURL(base, ep)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if ep.RequestBody != nil {
		payload, err := json.Marshal(mock.GenerateExample(&ep.RequestBody.Schema))
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(ep.Method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if ep.RequestBody != nil {
		ct := ep.RequestBody.ContentType
		if ct == "" {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	return req, nil
}

func endpointURL(base string, ep ir.Endpoint) (string, http.Header, error) {
	path := ep.Path
	if path == "" || !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	header := make(http.Header)
	query := url.Values{}
	for _, p := range ep.Parameters {
		value := exampleString(p.Schema)
		switch p.In {
		case "path":
			if value == "" {
				value = "0"
			}
			needle := "{" + p.Name + "}"
			if !strings.Contains(path, needle) {
				return "", nil, fmt.Errorf("path parameter %q missing from %s", p.Name, ep.Path)
			}
			path = strings.ReplaceAll(path, needle, url.PathEscape(value))
		case "query":
			query.Set(p.Name, value)
		case "header":
			header.Set(p.Name, value)
		}
	}
	raw := base + path
	if enc := query.Encode(); enc != "" {
		raw += "?" + enc
	}
	return raw, header, nil
}

func exampleString(schema ir.TypeRef) string {
	switch v := mock.GenerateExample(&schema).(type) {
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return ""
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func validateResponse(ep ir.Endpoint, res *http.Response, body []byte) error {
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	success, ok := lowest2xx(ep)
	if !ok {
		return fmt.Errorf("status %d with no 2xx response in the spec", res.StatusCode)
	}
	if success.Schema != nil || len(bytes.TrimSpace(body)) > 0 {
		if !json.Valid(body) {
			return fmt.Errorf("body is not valid JSON")
		}
	}
	want := success.ContentType
	if success.Schema != nil && want == "" {
		want = "application/json"
	}
	got := res.Header.Get("Content-Type")
	if mediaType(want) != mediaType(got) {
		return fmt.Errorf("content-type %q, want %q", got, want)
	}
	return nil
}

func lowest2xx(ep ir.Endpoint) (ir.Response, bool) {
	var best ir.Response
	found := false
	for _, resp := range ep.Responses {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if !found || resp.StatusCode < best.StatusCode {
			best = resp
			found = true
		}
	}
	return best, found
}

func mediaType(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(v)
	if err != nil {
		return strings.ToLower(v)
	}
	return strings.ToLower(mt)
}
