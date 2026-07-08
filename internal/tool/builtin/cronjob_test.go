package builtin

import (
	"testing"
)

func TestCronJobSchema(t *testing.T) {
	cj := cronJob{}
	if cj.Name() != "cronjob" {
		t.Fatalf("name: %s", cj.Name())
	}
	if cj.ReadOnly() {
		t.Fatal("cronjob should not be read-only")
	}
}

func TestCronJobListEmpty(t *testing.T) {
	cj := cronJob{}
	result, err := cj.Execute(t.Context(), []byte(`{"action":"list"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result != "No cron jobs." {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestCronJobCreateDelete(t *testing.T) {
	cj := cronJob{}
	_, err := cj.Execute(t.Context(), []byte(`{"action":"create","id":"test-job","name":"Test","prompt":"echo hello","schedule":"every 30m"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = cj.Execute(t.Context(), []byte(`{"action":"delete","id":"test-job"}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestCronJobInvalidAction(t *testing.T) {
	cj := cronJob{}
	_, err := cj.Execute(t.Context(), []byte(`{"action":"invalid"}`))
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}
