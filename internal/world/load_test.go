package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dataDir = "../../data"

// copyData duplicates the real world into a temporary directory, so a test can break
// a file without ever touching data/.
func copyData(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dataDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func breakFile(t *testing.T, dir, file, old, new string) {
	t.Helper()
	p := filepath.Join(dir, file)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, old) {
		t.Fatalf("%s: pattern %q not found", file, old)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(s, old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealWorldLoadsAndValidates(t *testing.T) {
	w, err := Load(dataDir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("validate:\n%v", err)
	}
	t.Logf("rooms=%d npcs=%d monsters=%d items=%d moves=%d quests=%d",
		len(w.Rooms), len(w.NPCs), len(w.Monsters), len(w.Items), len(w.Moves), len(w.Quests))
}

func TestLoadRejectsDuplicateID(t *testing.T) {
	dir := copyData(t)
	breakFile(t, dir, "zones.json", `"id": "zone.verdant_plains"`, `"id": "zone.village"`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for a duplicate id")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("-> %v", err)
}

func TestLoadRejectsUnknownField(t *testing.T) {
	dir := copyData(t)
	breakFile(t, dir, "monsters.json", `"xp": 12`, `"xpp": 12`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("-> %v", err)
}

func TestValidateRejectsBrokenExit(t *testing.T) {
	dir := copyData(t)
	breakFile(t, dir, "rooms.json", `"north": "loc.tavern"`, `"north": "loc.tavernn"`)
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	err = w.Validate()
	if err == nil {
		t.Fatal("expected a validation error for a broken exit")
	}
	if !strings.Contains(err.Error(), "leads to unknown room") {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("-> %v", err)
}
