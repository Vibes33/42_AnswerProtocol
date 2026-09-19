package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func Load(dir string) (*World, error) {
	w := &World{}
	var err error

	if err = decodeFile(filepath.Join(dir, "game.json"), &w.Config); err != nil {
		return nil, err
	}
	if err = decodeFile(filepath.Join(dir, "types.json"), &w.Types); err != nil {
		return nil, err
	}

	if w.Moves, err = loadList(filepath.Join(dir, "moves.json"), func(m *Move) string { return m.ID }); err != nil {
		return nil, err
	}
	if w.Statuses, err = loadList(filepath.Join(dir, "statuses.json"), func(s *StatusDef) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Archetypes, err = loadList(filepath.Join(dir, "archetypes.json"), func(s *Archetype) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Characters, err = loadList(filepath.Join(dir, "characters.json"), func(s *Character) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Items, err = loadList(filepath.Join(dir, "items.json"), func(s *Item) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.NPCs, err = loadList(filepath.Join(dir, "npcs.json"), func(s *NPC) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Monsters, err = loadList(filepath.Join(dir, "monsters.json"), func(s *Monster) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Zones, err = loadList(filepath.Join(dir, "zones.json"), func(s *Zone) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Rooms, err = loadList(filepath.Join(dir, "rooms.json"), func(s *Room) string { return s.ID }); err != nil {
		return nil, err
	}
	if w.Quests, err = loadList(filepath.Join(dir, "quests.json"), func(s *Quest) string { return s.ID }); err != nil {
		return nil, err
	}

	return w, nil
}

func loadList[T any](path string, id func(T) string) (map[string]T, error) {
	var list []T
	if err := decodeFile(path, &list); err != nil {
		return nil, err
	}

	byID := make(map[string]T, len(list))
	for _, v := range list {
		key := id(v)
		if key == "" {
			return nil, fmt.Errorf("%s: entry without id", path)
		}
		if _, exists := byID[key]; exists {
			return nil, fmt.Errorf("%s: duplicate id %q", path, key)
		}
		byID[key] = v
	}
	return byID, nil
}

func decodeFile(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
