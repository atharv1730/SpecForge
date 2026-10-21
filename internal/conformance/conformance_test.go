package conformance

import (
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/atharv1730/SpecForge/internal/ir"
	"github.com/atharv1730/SpecForge/internal/loader"
)

func TestPetstore(t *testing.T) {
	t.Chdir(repoRoot(t))

	doc, err := loader.Load("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	api, err := loader.Transform(doc)
	if err != nil {
		t.Fatal(err)
	}

	report, err := Run(api, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed != len(api.Endpoints) || report.Failed != 0 || len(report.Failures) != 0 {
		t.Fatalf("report = %+v, endpoints = %d", report, len(api.Endpoints))
	}
	if report.Passed != 3 {
		t.Fatalf("passed = %d", report.Passed)
	}
}

func TestFailureDetail(t *testing.T) {
	api := &ir.API{Endpoints: []ir.Endpoint{{
		Method:      http.MethodGet,
		Path:        "/missing-success",
		OperationID: "noSuccess",
		Responses:   []ir.Response{{StatusCode: http.StatusNotFound, ContentType: "application/json"}},
	}}}

	report, err := Run(api, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed != 0 || report.Failed != 1 || len(report.Failures) != 1 {
		t.Fatalf("report = %+v", report)
	}
	got := report.Failures[0]
	if got.Method != http.MethodGet || got.Path != "/missing-success" || got.OperationID != "noSuccess" {
		t.Fatalf("failure = %+v", got)
	}
	if !strings.Contains(got.Message, "status") {
		t.Fatalf("message = %q", got.Message)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}
