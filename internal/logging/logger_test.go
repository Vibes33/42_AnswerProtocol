package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWritesOneJSONObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)
	l.Info("command", Fields{"player": "alice", "verb": "MOVE"})
	l.Warn("command_flood", Fields{"player": "bob", "count": 42})
	l.Error("world_invalid", Fields{"reason": "broken exit"})
	l.Close()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), buf.String())
	}

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line is not valid JSON: %v", err)
	}
	for _, key := range []string{"ts", "level", "event", "player", "verb"} {
		if _, ok := first[key]; !ok {
			t.Errorf("missing field %q in %s", key, lines[0])
		}
	}
	if first["level"] != "INFO" || first["event"] != "command" {
		t.Errorf("unexpected level or event: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"level":"WARN"`) || !strings.Contains(lines[2], `"level":"ERROR"`) {
		t.Errorf("levels not preserved: %q %q", lines[1], lines[2])
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	var l *Logger
	l.Info("ignored", nil)
	l.Close()
	if l.Dropped() != 0 {
		t.Error("a nil logger should report no drop")
	}
}
