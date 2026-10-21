package mock

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atharv1730/SpecForge/internal/ir"
)

// NewServer routes each endpoint to its lowest 2xx response and an example body.
// Paths that are not in the API get a 404.
func NewServer(api *ir.API, port int) *http.Server {
	mux := http.NewServeMux()
	if api != nil {
		for _, ep := range api.Endpoints {
			if ep.Method == "" || ep.Path == "" {
				continue
			}
			mux.HandleFunc(ep.Method+" "+ep.Path, handler(successResponse(ep)))
		}
	}
	return &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
}

// successResponse is the lowest 2xx response, or nil when the endpoint has none.
func successResponse(ep ir.Endpoint) *ir.Response {
	var best *ir.Response
	for i := range ep.Responses {
		resp := &ep.Responses[i]
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if best == nil || resp.StatusCode < best.StatusCode {
			best = resp
		}
	}
	if best == nil {
		return nil
	}
	copied := *best
	return &copied
}

func handler(resp *ir.Response) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if resp == nil {
			http.Error(w, "no successful response", http.StatusInternalServerError)
			return
		}
		if resp.Schema == nil {
			if resp.ContentType != "" {
				w.Header().Set("Content-Type", resp.ContentType)
			}
			w.WriteHeader(resp.StatusCode)
			return
		}
		body, err := json.Marshal(GenerateExample(resp.Schema))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ct := resp.ContentType
		if ct == "" {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	}
}
