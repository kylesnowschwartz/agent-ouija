package settings

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// assertFileBytes fails the test unless the file at path holds exactly want.
func assertFileBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("settings.json mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// orderedSettings is 2-space-indented standard JSON with keys out of
// alphabetical order at every level, HTML-significant characters, non-ASCII
// text, escape sequences and numbers whose text a float64 round trip would
// change.
const orderedSettings = `{
  "model": "opus",
  "env": {
    "ZETA": "1",
    "R&D": "a < b && c > d",
    "ALPHA": "a"
  },
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "existing-tool stop"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "guard <cmd> & log",
            "timeout": 30
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "hud",
    "padding": 0
  },
  "costThreshold": 0.015,
  "greeting": "Kia ora — café ☕",
  "permissions": {
    "deny": [],
    "allow": [
      "Bash(ls:*)"
    ]
  },
  "escaped": "é\n<",
  "tiny": 1e-05,
  "large": 12345678901234567890
}
`

func TestRegisterHooks_KeepsKeyOrderAndValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, orderedSettings)

	added, err := RegisterHooks(path, []HookCommand{
		{Event: "Stop", Command: "tool", Args: []string{"hook", "stop"}},
		{Event: "SessionStart", Command: "tool", Args: []string{"hook", "session-start"}, Async: true},
	})
	if err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("added = %v, want [Stop SessionStart]", added)
	}

	assertFileBytes(t, path, `{
  "model": "opus",
  "env": {
    "ZETA": "1",
    "R&D": "a < b && c > d",
    "ALPHA": "a"
  },
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "existing-tool stop"
          }
        ]
      },
      {
        "hooks": [
          {
            "type": "command",
            "command": "tool",
            "args": [
              "hook",
              "stop"
            ]
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "guard <cmd> & log",
            "timeout": 30
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "tool",
            "args": [
              "hook",
              "session-start"
            ],
            "async": true
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "hud",
    "padding": 0
  },
  "costThreshold": 0.015,
  "greeting": "Kia ora — café ☕",
  "permissions": {
    "deny": [],
    "allow": [
      "Bash(ls:*)"
    ]
  },
  "escaped": "é\n<",
  "tiny": 1e-05,
  "large": 12345678901234567890
}
`)
}

func TestRegisterHooks_NoOpLeavesFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, orderedSettings)
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	// Already registered in legacy shell-string form.
	added, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "existing-tool", Args: []string{"stop"}}})
	if err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}
	if added != nil {
		t.Errorf("added = %v, want nil", added)
	}

	assertFileBytes(t, path, orderedSettings)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("mtime = %v, want %v (file must not be rewritten)", info.ModTime(), past)
	}
}

func TestRegisterHooks_MissingFileCreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")

	if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "/opt/a&b/hook.sh"}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	assertFileBytes(t, path, `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/opt/a&b/hook.sh"
          }
        ]
      }
    ]
  }
}
`)
}

func TestRegisterHooks_MissingHooksKeyGoesLast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{
  "model": "opus",
  "env": {
    "B": "2",
    "A": "1"
  }
}
`)

	if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "tool"}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	assertFileBytes(t, path, `{
  "model": "opus",
  "env": {
    "B": "2",
    "A": "1"
  },
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "tool"
          }
        ]
      }
    ]
  }
}
`)
}

func TestRegisterHooks_ReplacesNonObjectHooksInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"model": "opus", "hooks": "unexpected", "theme": "dark"}`)

	if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "tool"}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	assertFileBytes(t, path, `{
  "model": "opus",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "tool"
          }
        ]
      }
    ]
  },
  "theme": "dark"
}
`)
}

func TestRegisterHooks_ReplacesNonArrayEventInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, `{"hooks": {"Stop": {"bad": true}, "PreToolUse": []}}`)

	if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "tool"}}); err != nil {
		t.Fatalf("RegisterHooks: %v", err)
	}

	assertFileBytes(t, path, `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "tool"
          }
        ]
      }
    ],
    "PreToolUse": []
  }
}
`)
}

func TestRegisterHooks_RejectsNonObjectFile(t *testing.T) {
	for _, content := range []string{`[]`, `null`, `"settings"`, `{"a": 1} {"b": 2}`} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			writeFile(t, path, content)

			if _, err := RegisterHooks(path, []HookCommand{{Event: "Stop", Command: "tool"}}); err == nil {
				t.Error("RegisterHooks succeeded, want a parse error")
			}
			assertFileBytes(t, path, content)
		})
	}
}
