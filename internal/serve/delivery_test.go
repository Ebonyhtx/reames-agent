package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/tool"
	"reames-agent/internal/workspacelease"
)

func initServeDeliveryRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	runServeDeliveryGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runServeDeliveryGit(t, repo, "add", ".")
	runServeDeliveryGit(t, repo, "commit", "-m", "base")
	return repo
}

func runServeDeliveryGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "core.longpaths=true", "-c", "user.name=Reames Test", "-c", "user.email=test@example.invalid", "-C", dir}
	cmd := exec.Command("git", append(base, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestServeDeliveryEndpointsProjectControllerLifecycle(t *testing.T) {
	repo := initServeDeliveryRepo(t)
	managed := t.TempDir()
	leaseDir := t.TempDir()
	sessionDir := t.TempDir()
	parentSession := "parent-session"
	coordinator := agent.NewSubagentWorkspaceCoordinator(managed, leaseDir, func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return agent.SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store := agent.NewSubagentStore(filepath.Join(sessionDir, "subagents")).WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(agent.SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: parentSession, Registry: tool.NewRegistry(), Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.ExecutionRoot, "serve.txt"), []byte("serve\n"), 0o644); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := store.SaveCompleted(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()
	lease, err := workspacelease.New(repo, leaseDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		SessionDir:     sessionDir,
		SessionPath:    filepath.Join(sessionDir, parentSession+".jsonl"),
		WorkspaceRoot:  repo,
		WorkspaceLease: lease,
		SubagentStore:  store,
		Sink:           bc,
	})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/deliveries")
	if err != nil {
		t.Fatal(err)
	}
	var list []control.SubagentDeliveryView
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(list) != 1 || list[0].Ref != run.Ref {
		t.Fatalf("GET /deliveries status=%d body=%+v", resp.StatusCode, list)
	}

	resp, err = http.Get(srv.URL + "/delivery?ref=" + run.Ref)
	if err != nil {
		t.Fatal(err)
	}
	var preview control.SubagentDeliveryView
	if err := json.NewDecoder(resp.Body).Decode(&preview); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(preview.Changes) != 1 || preview.Changes[0].Path != "serve.txt" {
		t.Fatalf("GET /delivery status=%d body=%+v", resp.StatusCode, preview)
	}

	for _, op := range []struct {
		name string
		want agent.SubagentDeliveryStatus
	}{
		{name: "apply", want: agent.SubagentDeliveryApplied},
		{name: "rollback", want: agent.SubagentDeliveryRolledBack},
		{name: "reject", want: agent.SubagentDeliveryRejected},
	} {
		body, _ := json.Marshal(map[string]string{"ref": run.Ref, "op": op.name})
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/delivery", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var view control.SubagentDeliveryView
		if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || view.Status != op.want {
			t.Fatalf("POST /delivery %s status=%d body=%+v", op.name, resp.StatusCode, view)
		}
	}

	resp, err = http.Post(srv.URL+"/delivery", "text/plain", strings.NewReader(`{"ref":"x","op":"reject"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("cross-site compatible POST status=%d, want 415", resp.StatusCode)
	}
}
