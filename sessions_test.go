package sessions_test

import (
	"errors"
	"testing"
	"time"

	sessions "github.com/kylesnowschwartz/agent-ouija"
	"github.com/kylesnowschwartz/agent-ouija/sessionstest"
)

func ref(id string, mod time.Time) sessions.SessionRef {
	return sessions.SessionRef{ID: id, ModTime: mod}
}

func TestRegistryDiscover_MergesAndSorts(t *testing.T) {
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	a := &sessionstest.FakeProvider{ProviderName: "alpha", Refs: []sessions.SessionRef{
		ref("a-old", base.Add(1*time.Hour)),
		ref("a-new", base.Add(4*time.Hour)),
	}}
	b := &sessionstest.FakeProvider{ProviderName: "beta", Refs: []sessions.SessionRef{
		ref("b-mid", base.Add(2*time.Hour)),
	}}

	r := sessions.NewRegistry(a, b)
	refs, err := r.Discover(sessions.Query{})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, x := range refs {
		got = append(got, x.Provider+"/"+x.ID)
	}
	want := []string{"alpha/a-new", "beta/b-mid", "alpha/a-old"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("merged order = %v, want %v", got, want)
		}
	}

	// Limit applies after the merge.
	refs, _ = r.Discover(sessions.Query{Limit: 2})
	if len(refs) != 2 || refs[0].ID != "a-new" {
		t.Errorf("Limit: got %+v", refs)
	}
}

func TestRegistryDiscover_BrokenProviderDoesNotHideOthers(t *testing.T) {
	boom := errors.New("boom")
	broken := &sessionstest.FakeProvider{ProviderName: "broken", DiscoverErr: boom}
	ok := &sessionstest.FakeProvider{ProviderName: "ok", Refs: []sessions.SessionRef{
		ref("x", time.Now()),
	}}

	r := sessions.NewRegistry(broken, ok)
	refs, err := r.Discover(sessions.Query{})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom surfaced", err)
	}
	if len(refs) != 1 || refs[0].ID != "x" {
		t.Errorf("healthy provider's results lost: %+v", refs)
	}
}

func TestRegistryProviderLookup(t *testing.T) {
	a := &sessionstest.FakeProvider{ProviderName: "alpha"}
	r := sessions.NewRegistry(a)
	if p, ok := r.Provider("alpha"); !ok || p.Name() != "alpha" {
		t.Error("lookup by name failed")
	}
	if _, ok := r.Provider("nope"); ok {
		t.Error("unknown name must report not-found")
	}
}
