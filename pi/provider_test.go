package pi_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/pi"
	"github.com/kylesnowschwartz/agent-ouija/pi/discover"
	"github.com/kylesnowschwartz/agent-ouija/pi/pidir"
	"github.com/kylesnowschwartz/agent-ouija/sessionstest"
)

func seed(t *testing.T, root pidir.Root, i int, cwd, name string, mod time.Time, last string) {
	t.Helper()
	dir := discover.SessionDirForCwd(root.SessionsDir(), cwd)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("s%d", i)
	path := filepath.Join(dir, mod.UTC().Format("2006-01-02T15-04-05-000Z")+"_"+id+".jsonl")
	body := fmt.Sprintf(`{"type":"session","version":3,"id":%q,"timestamp":%q,"cwd":%q}`+"\n", id, mod.UTC().Format(time.RFC3339Nano), cwd) +
		`{"type":"message","id":"u","parentId":null,"timestamp":"2026-07-01T00:00:00Z","message":{"role":"user","content":"hi"}}` + "\n" + last
	if name != "" {
		body += fmt.Sprintf(`{"type":"session_info","id":"n","parentId":"u","timestamp":"2026-07-01T00:00:00Z","name":%q}`+"\n", name)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestPiProviderConformance(t *testing.T) {
	sessionstest.Run(t, sessionstest.Harness{
		Make: func(t *testing.T, seeds []sessionstest.Seed) sessions.Provider {
			root := pidir.Root(filepath.Join(t.TempDir(), "agent"))
			for i, s := range seeds {
				seed(t, root, i, s.ProjectDir, s.Title, s.ModTime, "")
			}
			return pi.New(root)
		},
	})
}

func TestPiProvider_Ongoing(t *testing.T) {
	stop := `{"type":"message","id":"a","parentId":"u","timestamp":"2026-07-01T00:00:01Z","message":{"role":"assistant","content":[],"stopReason":"stop"}}` + "\n"
	tests := []struct {
		name string
		last string
		mod  time.Time
		want bool
	}{
		{"fresh working", "", time.Now(), true},
		{"stale working", "", time.Now().Add(-time.Hour), false},
		{"fresh done", stop, time.Now(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := pidir.Root(t.TempDir())
			seed(t, root, 1, "/work/proj", "", tt.mod, tt.last)
			refs, err := pi.New(root).Discover(sessions.Query{})
			if err != nil || len(refs) != 1 {
				t.Fatalf("refs = %+v, %v", refs, err)
			}
			if refs[0].Ongoing != tt.want {
				t.Errorf("Ongoing = %v, want %v", refs[0].Ongoing, tt.want)
			}
		})
	}
}
