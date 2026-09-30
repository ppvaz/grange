package reap

import (
	"os"
	"testing"
)

func TestStateRoundTripStaysDashboardReadable(t *testing.T) {
	dir := t.TempDir()
	want := State{CurrentStage: 3, TargetPath: "/src/shop", TargetStack: "Node.js + TypeScript", BuildDir: "/build", StartedAt: "2026-09-30T10:46:46-03:00"}
	if err := want.save(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(statePath(dir))
	if got := string(data); got != "CURRENT_STAGE=3\nTARGET_PATH=/src/shop\nTARGET_STACK=Node.js + TypeScript\nBUILD_DIR=/build\nSTARTED_AT=2026-09-30T10:46:46-03:00\n" {
		t.Errorf("unexpected .ike-state layout:\n%s", got)
	}
	got, err := loadState(dir)
	if err != nil || got != want {
		t.Errorf("loadState = %+v, %v; want %+v", got, err, want)
	}
}

func TestLoadStateToleratesQuotesFromOlderWriters(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(statePath(dir), []byte("CURRENT_STAGE=0\nTARGET_STACK=\"Go + Postgres\"\n"), 0o644)
	st, err := loadState(dir)
	if err != nil || st.CurrentStage != 0 || st.TargetStack != "Go + Postgres" {
		t.Errorf("got %+v, %v", st, err)
	}
}
