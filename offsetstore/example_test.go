package offsetstore_test

import (
	"github.com/kylesnowschwartz/agent-ouija/offsetstore"
)

// The per-tick pattern: read only new bytes, restore extraction state,
// persist both. Bump the schema version whenever extraction semantics
// change — stale snapshots are then discarded automatically.
func Example() {
	const extractionSchemaVersion = 3

	store := offsetstore.New("/path/to/state-dir", extractionSchemaVersion)
	lines, err := store.ReadIncremental("/path/to/transcript.jsonl")
	if err != nil {
		return // first tick may race session creation; render without transcript data
	}
	if snap := store.LoadSnapshot(); snap != nil {
		_ = snap // restore accumulated state from the previous tick
	}
	for _, line := range lines {
		_ = line // parse with transcript.ParseEntryLenient, accumulate
	}
	store.SetSnapshot(nil) // marshal the accumulated state here
	_ = store.Save("/path/to/transcript.jsonl")
}
