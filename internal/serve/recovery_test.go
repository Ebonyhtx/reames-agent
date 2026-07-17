package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/repair"
)

func TestRecoveryEndpointProjectsControllerReport(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	ctrl := control.New(control.Options{WorkspaceRoot: t.TempDir()})
	srv := httptest.NewServer(New(ctrl, NewBroadcaster(), config.ServeConfig{}).Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/recovery")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var report repair.Report
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("report = %+v", report)
	}
}
