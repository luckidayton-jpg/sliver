package evilginx

import (
	"fmt"
	"os"
	"testing"
)

// TestManualStartDebug is a manual smoke test: XAV_SLIVER_MANUAL=1 go test
// -tags 'server go_sqlite' -run TestManualStartDebug -v ./evilginx/
func TestManualStartDebug(t *testing.T) {
	if os.Getenv("XAV_SLIVER_MANUAL") == "" {
		t.Skip("manual debug test: set XAV_SLIVER_MANUAL=1 to run")
	}
	appDir := t.TempDir()
	port := uint16(31339)
	b := NewBridge(&SliverConfig{Host: "127.0.0.1", Port: port, AppDir: appDir})
	if err := b.Start(); err != nil {
		t.Fatalf("bridge start failed: %v", err)
	}
	defer b.Stop()

	st := b.Status()
	fmt.Printf("STATUS: running=%v sessions=%d jobs=%d err=%q\n", st.Running, st.Sessions, st.Jobs, st.Error)
	sess, err := b.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	fmt.Printf("SESSIONS: %d\n", len(sess))
	jobs, err := b.ListJobs()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	fmt.Printf("JOBS: %d\n", len(jobs))
}