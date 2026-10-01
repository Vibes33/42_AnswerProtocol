package game

import (
	"fmt"
	"strings"

	"tap/internal/logging"
	"tap/internal/world"
)

const errNoQuestAvailable = "ERR 406 NO_QUEST_AVAILABLE"

// questReply follows the shape of RFC 42TAP 5.4.7.
type questReply struct {
	QuestID     string `json:"quest_id"`
	Description string `json:"description"`
	Reward      string `json:"reward"`
	Status      string `json:"status"`
	Progress    string `json:"progress,omitempty"`
}

type questEntry struct {
	QuestID  string `json:"quest_id"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
}

// findNPC looks for a peaceful NPC of the room, by id or display name.
func (h *Hub) findNPC(room, query string) *world.NPC {
	q := normalizeQuery(query)
	for _, id := range h.world.Rooms[room].NPCs {
		npc := h.world.NPCs[id]
		if q == strings.ToLower(npc.ID) || q == strings.ToLower(npc.Name) || "npc."+q == strings.ToLower(npc.ID) {
			return npc
		}
	}
	return nil
}

// talk walks through an NPC's dialogue lines, one per TALK, looping back to the first.
// Monsters answer with their taunt when they have one.
func (h *Hub) talk(c *Client, query string) {
	if strings.TrimSpace(query) == "" {
		c.send(errMissingArgument)
		return
	}
	if npc := h.findNPC(c.Room, query); npc != nil {
		if len(npc.Dialogue) == 0 {
			c.send("OK ...")
			return
		}
		i := c.talkIndex[npc.ID] % len(npc.Dialogue)
		c.talkIndex[npc.ID] = i + 1
		c.send("OK " + npc.Dialogue[i])
		return
	}
	if m := h.findMonster(c.Room, query); m != nil && len(m.Def.Dialogue) > 0 {
		c.send("OK " + m.Def.Dialogue[0])
		return
	}
	c.send(errNPCNotFound)
}

// creditKill advances every active "defeat" quest aimed at this monster.
func (h *Hub) creditKill(c *Client, monsterID string) []string {
	var lines []string
	for _, id := range sortedKeys(c.Quests) {
		p := c.Quests[id]
		q := h.world.Quests[id]
		if p.Completed || q.Objective.Kind != world.ObjectiveDefeat || q.Objective.Target != monsterID {
			continue
		}
		p.Count = min(p.Count+1, q.Objective.Count)
		lines = append(lines, fmt.Sprintf("%s: %s %d/%d", c.Name, q.Name, p.Count, q.Objective.Count))
	}
	return lines
}

// progressOf reports how far a quest has come, counting inventory for fetch and deliver.
func (h *Hub) progressOf(c *Client, q *world.Quest, p *QuestProgress) (int, int) {
	if q.Objective.Kind == world.ObjectiveDefeat {
		return p.Count, q.Objective.Count
	}
	held := 0
	for _, inst := range h.inventoryOf(c) {
		if inst.Def.ID == q.Objective.Target {
			held++
		}
	}
	return min(held, q.Objective.Count), q.Objective.Count
}

func (h *Hub) canStart(c *Client, q *world.Quest) bool {
	if c.Quests[q.ID] != nil || c.Level < q.LevelRequired {
		return false
	}
	for _, pre := range q.Prerequisites {
		if p := c.Quests[pre]; p == nil || !p.Completed {
			return false
		}
	}
	return true
}

// quest is both the way to receive a quest and the way to hand it in: the server completes
// any finished objective the NPC is responsible for, then offers the next available quest.
func (h *Hub) quest(c *Client, query string) {
	if strings.TrimSpace(query) == "" {
		c.send(errMissingArgument)
		return
	}
	npc := h.findNPC(c.Room, query)
	if npc == nil {
		c.send(errNPCNotFound)
		return
	}

	// Quests this NPC can take back: its own, plus deliveries addressed to it.
	for _, id := range sortedKeys(c.Quests) {
		p := c.Quests[id]
		q := h.world.Quests[id]
		turnIn := q.Objective.TurnIn
		if turnIn == "" {
			turnIn = q.Giver
		}
		if p.Completed || turnIn != npc.ID {
			continue
		}
		done, total := h.progressOf(c, q, p)
		if done < total {
			continue
		}
		h.completeQuest(c, q, p)
		h.sendJSON(c, questReply{
			QuestID: q.ID, Description: q.Description, Reward: rewardText(q.Reward),
			Status: "completed", Progress: fmt.Sprintf("%d/%d", total, total),
		})
		return
	}

	// Otherwise, offer something new, or report an ongoing quest.
	for _, id := range npc.Quests {
		q := h.world.Quests[id]
		if h.canStart(c, q) {
			c.Quests[q.ID] = &QuestProgress{}
			h.log.Info("quest_started", logging.Fields{"player": c.Name, "quest": q.ID})
			h.sendJSON(c, questReply{
				QuestID: q.ID, Description: q.Description, Reward: rewardText(q.Reward),
				Status: "active", Progress: fmt.Sprintf("0/%d", q.Objective.Count),
			})
			return
		}
		if p := c.Quests[id]; p != nil && !p.Completed {
			done, total := h.progressOf(c, q, p)
			h.sendJSON(c, questReply{
				QuestID: q.ID, Description: q.Description, Reward: rewardText(q.Reward),
				Status: "active", Progress: fmt.Sprintf("%d/%d", done, total),
			})
			return
		}
	}
	c.send(errNoQuestAvailable)
}

// completeQuest consumes the required items, hands out the rewards and marks the quest done.
func (h *Hub) completeQuest(c *Client, q *world.Quest, p *QuestProgress) {
	if q.Objective.Kind != world.ObjectiveDefeat {
		left := q.Objective.Count
		for _, inst := range h.inventoryOf(c) {
			if left == 0 {
				break
			}
			if inst.Def.ID == q.Objective.Target {
				delete(h.items, inst.ID)
				left--
			}
		}
	}
	p.Completed = true
	c.Gold += q.Reward.Gold
	for _, stack := range q.Reward.Items {
		for i := 0; i < stack.Count; i++ {
			h.newItem(stack.Item, "", c)
		}
	}
	levelUps := h.grantXP(c, q.Reward.XP)
	h.log.Info("quest_completed", logging.Fields{
		"player": c.Name, "quest": q.ID, "xp": q.Reward.XP, "gold": q.Reward.Gold, "level_ups": levelUps,
	})
}

func (h *Hub) quests(c *Client) {
	entries := []questEntry{}
	for _, id := range sortedKeys(c.Quests) {
		p := c.Quests[id]
		q := h.world.Quests[id]
		done, total := h.progressOf(c, q, p)
		status := "active"
		if p.Completed {
			status, done = "completed", total
		}
		entries = append(entries, questEntry{
			QuestID: id, Status: status, Progress: fmt.Sprintf("%d/%d", done, total),
		})
	}
	h.sendJSON(c, entries)
}

func rewardText(r world.Reward) string {
	parts := []string{}
	if r.XP > 0 {
		parts = append(parts, fmt.Sprintf("%d xp", r.XP))
	}
	if r.Gold > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", r.Gold))
	}
	for _, stack := range r.Items {
		parts = append(parts, fmt.Sprintf("%d x %s", stack.Count, stack.Item))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
