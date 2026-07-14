package control

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/diff"
	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
)

type rewindCrashFixture struct {
	controller  *Controller
	agent       *agent.Agent
	path        string
	file        string
	transaction agent.RewindTransactionMeta
}

func newRewindCrashFixture(t *testing.T, mark bool) rewindCrashFixture {
	t.Helper()
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	turn := c.checkpoints.turn - 1
	boundary := c.checkpoints.bound[turn]
	c.checkpoints.mu.Unlock()
	digest, ok := c.checkpoints.transcriptDigest(turn)
	if !ok {
		t.Fatalf("checkpoint turn %d has no transcript digest", turn)
	}
	runtimeData, ok := c.checkpoints.runtime(turn)
	if !ok {
		t.Fatalf("checkpoint turn %d has no runtime projection", turn)
	}
	file := filepath.Join(c.workspaceRoot, "rewind-state.txt")
	if err := os.WriteFile(file, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.checkpoints.snapshot(diff.Change{Path: file, Kind: diff.Modify, OldText: "before"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	ag.SeedTodoState([]evidence.TodoItem{{Content: "future todo", Status: "in_progress"}})
	c.SetGoal("future goal")
	c.SetPlanMode(true)
	if err := c.goals.persistRuntime(c.goalRuntimeProjection()); err != nil {
		t.Fatal(err)
	}
	transaction := agent.RewindTransactionMeta{
		Turn: turn, Boundary: boundary, TranscriptDigest: digest,
		Runtime: runtimeData, IncludeCode: true,
		Phase: agent.RewindTransactionPrepared, StartedAt: time.Now().UTC(),
	}
	if mark {
		if err := agent.MarkSessionRewindTransaction(c.SessionPath(), transaction); err != nil {
			t.Fatal(err)
		}
	}
	return rewindCrashFixture{controller: c, agent: ag, path: c.SessionPath(), file: file, transaction: transaction}
}

func resumeRewindCrashFixture(t *testing.T, fixture rewindCrashFixture) *Controller {
	t.Helper()
	loaded, err := agent.LoadSession(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	freshAgent := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	resumed := New(Options{
		Executor: freshAgent, SessionDir: filepath.Dir(fixture.path), WorkspaceRoot: fixture.controller.workspaceRoot,
		Label: "test", DisableColdResumePrune: true,
	})
	resumed.Resume(loaded, fixture.path)
	return resumed
}

func assertRewindCrashRecovered(t *testing.T, resumed *Controller, fixture rewindCrashFixture) {
	t.Helper()
	data, err := os.ReadFile(fixture.file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("workspace after recovery = %q, want before", data)
	}
	if resumed.messageCount() != fixture.transaction.Boundary {
		t.Fatalf("message count after recovery = %d, want %d", resumed.messageCount(), fixture.transaction.Boundary)
	}
	if equal, _ := resumed.executor.Session().CompareTranscriptAnchor(fixture.transaction.Boundary, fixture.transaction.TranscriptDigest); !equal {
		t.Fatal("recovered transcript does not match rewind anchor")
	}
	if resumed.Goal() != "" || resumed.PlanMode() || len(resumed.Todos()) != 0 {
		t.Fatalf("runtime after recovery = goal:%q plan:%v todos:%+v", resumed.Goal(), resumed.PlanMode(), resumed.Todos())
	}
	state, ok := resumed.goals.readSessionState(fixture.path)
	if !ok || state.MessageCount != fixture.transaction.Boundary || state.TranscriptDigest != fixture.transaction.TranscriptDigest {
		t.Fatalf("runtime anchor after recovery = ok:%v count:%d digest:%q", ok, state.MessageCount, state.TranscriptDigest)
	}
	meta, ok, err := agent.LoadBranchMeta(fixture.path)
	if err != nil || !ok || meta.Rewind != nil {
		t.Fatalf("rewind marker after recovery = ok:%v err:%v marker:%+v", ok, err, meta.Rewind)
	}
	if resumed.checkpoints.has(fixture.transaction.Turn) {
		t.Fatalf("checkpoint turn %d survived committed rewind", fixture.transaction.Turn)
	}
}

func TestRewindTransactionRecoversPreparedIntent(t *testing.T) {
	fixture := newRewindCrashFixture(t, true)
	resumed := resumeRewindCrashFixture(t, fixture)
	assertRewindCrashRecovered(t, resumed, fixture)
}

func TestRewindTransactionRecoversAfterResourcesCommit(t *testing.T) {
	for _, retireBeforeCrash := range []bool{false, true} {
		name := "before_checkpoint_retirement"
		if retireBeforeCrash {
			name = "after_checkpoint_retirement"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newRewindCrashFixture(t, true)
			if _, _, err := fixture.controller.applyRewindResources(fixture.path, fixture.transaction); err != nil {
				t.Fatal(err)
			}
			if err := agent.AdvanceSessionRewindTransaction(fixture.path, fixture.transaction); err != nil {
				t.Fatal(err)
			}
			if retireBeforeCrash {
				if err := fixture.controller.checkpoints.truncateFrom(fixture.transaction.Turn); err != nil {
					t.Fatal(err)
				}
			}
			resumed := resumeRewindCrashFixture(t, fixture)
			assertRewindCrashRecovered(t, resumed, fixture)
		})
	}
}

func TestRewindAPIPersistenceFailuresRemainColdRecoverable(t *testing.T) {
	tests := []struct {
		name      string
		inject    func(*Controller)
		wantPhase agent.RewindTransactionPhase
	}{
		{
			name: "resources_phase",
			inject: func(c *Controller) {
				c.rewindAdvance = func(string, agent.RewindTransactionMeta) error {
					return os.ErrPermission
				}
			},
			wantPhase: agent.RewindTransactionPrepared,
		},
		{
			name: "final_clear",
			inject: func(c *Controller) {
				c.rewindClear = func(string, agent.RewindTransactionMeta) error {
					return os.ErrPermission
				}
			},
			wantPhase: agent.RewindTransactionResourcesApplied,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRewindCrashFixture(t, false)
			test.inject(fixture.controller)
			if err := fixture.controller.Rewind(fixture.transaction.Turn, RewindBoth); err == nil {
				t.Fatal("Rewind succeeded despite injected transaction persistence failure")
			}
			meta, ok, err := agent.LoadBranchMeta(fixture.path)
			if err != nil || !ok || meta.Rewind == nil || meta.Rewind.Phase != test.wantPhase {
				t.Fatalf("pending rewind = ok:%v err:%v marker:%+v, want phase %q", ok, err, meta.Rewind, test.wantPhase)
			}
			resumed := resumeRewindCrashFixture(t, fixture)
			assertRewindCrashRecovered(t, resumed, fixture)
		})
	}
}
