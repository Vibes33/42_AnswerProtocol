package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// command envoie une commande tapée ou cliquée par le joueur et l'affiche dans les logs.
func (g *gui) command(line string) {
	g.logs.add("» " + line)
	g.send(line)
}

// send envoie une commande sans l'afficher (rafraîchissements automatiques comme LOOK).
func (g *gui) send(line string) {
	if g.cl == nil {
		return
	}
	if err := g.cl.send(line); err != nil {
		g.logs.add("✗ envoi impossible : " + err.Error())
	}
}

func (g *gui) withTarget(cmd string) {
	target := strings.TrimSpace(g.target.Text)
	if target == "" {
		g.logs.add("✗ " + cmd + " : choisis une cible dans une liste ou tape-la dans le champ « cible »")
		return
	}
	g.command(cmd + " " + target)
}

func (g *gui) sendChat() {
	msg := strings.TrimSpace(g.chatEntry.Text)
	if msg == "" {
		return
	}
	g.chatEntry.SetText("")
	g.command("CHAT " + chatScopes[g.chatTabs.SelectedIndex()] + " " + msg)
}

// refreshAll redemande tout l'état affiché au serveur.
func (g *gui) refreshAll() {
	for _, cmd := range []string{"LOOK", "INVENTORY", "STATUS", "QUESTS", "WHO"} {
		g.send(cmd)
	}
}

// onLine traite chaque ligne reçue du serveur. Elle tourne dans la goroutine de l'interface.
func (g *gui) onLine(line string) {
	if !g.cl.greeted {
		g.cl.greeted = true
		if strings.HasPrefix(line, "ERR") {
			g.loginErr.SetText("Connexion refusée : " + line)
			return
		}
		g.logs.add("« " + line)
		return
	}
	if rest, ok := strings.CutPrefix(line, "EVT "); ok {
		g.onEvent(rest)
		return
	}

	cmd := g.cl.popCommand()
	verb, arg, _ := strings.Cut(cmd, " ")
	if rest, isErr := strings.CutPrefix(line, "ERR "); isErr {
		g.logs.add(fmt.Sprintf("✗ %s : %s", cmd, rest))
		if verb == "CONNECT" {
			g.loginErr.SetText("Erreur : " + rest)
		}
		return
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "OK"))
	g.onReply(verb, arg, data)
}

// onReply met à jour l'interface selon la commande à laquelle le serveur vient de répondre OK.
func (g *gui) onReply(verb, arg, data string) {
	switch verb {
	case "CONNECT":
		g.enterGame()
	case "LOOK":
		g.applyLook(data)
	case "MOVE":
		g.logs.add("→ " + data)
		g.dialogue.SetText("")
		g.send("LOOK")
	case "TAKE", "DROP":
		g.logs.add("✓ " + data)
		g.send("LOOK")
		g.send("INVENTORY")
	case "INVENTORY":
		g.inventory = nil
		if err := json.Unmarshal([]byte(data), &g.inventory); err != nil {
			g.logs.add("« INVENTORY " + data)
		}
		refreshList(g.invList)
	case "TALK":
		g.applyTalk(arg, data)
	case "ATTACK":
		g.applyAttack(data)
		g.send("STATUS")
		g.send("LOOK")
	case "STATUS":
		g.applyStatus(data)
	case "QUEST":
		g.logs.add("« QUEST " + formatQuest(data))
		g.send("QUESTS")
	case "QUESTS":
		g.applyQuests(data)
	case "WHO":
		g.applyWho(data)
	case "GROUP":
		g.applyGroup(arg, data)
	case "QUIT":
		g.quitting = true
		g.app.Quit()
	case "CHAT":
		// Le message revient sous forme d'événement EVT ... CHAT : rien à faire ici.
	default:
		g.logs.add("« " + strings.TrimSpace("OK "+data))
	}
}

// onEvent traite les messages asynchrones « EVT <catégorie> <type> <données> ».
func (g *gui) onEvent(evt string) {
	parts := strings.SplitN(evt, " ", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	category, kind, rest := parts[0], parts[1], parts[2]

	switch {
	case kind == "CHAT":
		author, msg, _ := strings.Cut(rest, " ")
		if view, ok := g.chats[category]; ok {
			view.add(author + " : " + msg)
		}
	case category == "STATS":
		if n, ok := counter(rest, "players="); ok {
			g.serverCount = n
			g.updateCounters()
		}
	case category == "GROUP":
		g.onGroupEvent(kind, rest)
	case category == "ROOM":
		// PRESENCE ENTER/LEAVE, et les extensions de notre serveur (ITEM, COMBAT, SPAWN) :
		// on note l'événement et on recharge la salle pour garder la vue à jour.
		g.logs.add("• " + evt)
		g.send("LOOK")
	default:
		g.logs.add("• " + evt)
	}
}

func (g *gui) onGroupEvent(kind, who string) {
	g.logs.add("• GROUP " + kind + " " + who)
	switch kind {
	case "INVITE":
		dialog.ShowConfirm("Invitation de groupe",
			who+" t'invite dans son groupe. Le rejoindre ?",
			func(yes bool) {
				if yes {
					g.command("GROUP JOIN " + who)
				}
			}, g.win)
	case "JOIN":
		g.chats["GROUP"].add("* " + who + " a rejoint le groupe")
	case "LEAVE":
		g.chats["GROUP"].add("* " + who + " a quitté le groupe")
		if strings.EqualFold(who, g.name) {
			g.groupLabel.SetText("Groupe : aucun")
		}
	}
}

func (g *gui) applyLook(data string) {
	var look struct {
		Room struct {
			ID          string            `json:"id"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Exits       map[string]string `json:"exits"`
		} `json:"room"`
		Players []string `json:"players"`
		Items   []string `json:"items"`
		NPCs    []string `json:"npcs"`
	}
	if err := json.Unmarshal([]byte(data), &look); err != nil {
		g.logs.add("✗ LOOK illisible : " + err.Error())
		return
	}

	g.roomTitle.SetText(look.Room.Name)
	g.roomDesc.SetText(look.Room.Description)
	g.players, g.items, g.npcs = look.Players, look.Items, look.NPCs
	refreshList(g.playersList)
	refreshList(g.itemsList)
	refreshList(g.npcsList)
	g.roomCount = len(look.Players)
	g.updateCounters()

	// Un bouton par sortie, triés pour garder un ordre stable.
	dirs := make([]string, 0, len(look.Room.Exits))
	for dir := range look.Room.Exits {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	g.exits.RemoveAll()
	for _, dir := range dirs {
		g.exits.Add(widget.NewButton(dir, func() { g.command("MOVE " + dir) }))
	}
	// La zone qui défile ne voit pas que son contenu a changé de taille : on la relance.
	g.roomInfo.Refresh()
}

// applyTalk accepte les deux formats : texte brut (RFC) ou JSON {"npc", "dialogue"} (sujet),
// pour rester compatible avec les serveurs des autres groupes.
func (g *gui) applyTalk(npc, data string) {
	var reply struct {
		NPC      string `json:"npc"`
		Dialogue string `json:"dialogue"`
	}
	if json.Unmarshal([]byte(data), &reply) == nil && reply.Dialogue != "" {
		npc, data = reply.NPC, reply.Dialogue
	}
	g.dialogue.SetText(npc + " : « " + data + " »")
	g.logs.add("💬 " + npc + " : " + data)
}

func (g *gui) applyAttack(data string) {
	var reply map[string]any
	if err := json.Unmarshal([]byte(data), &reply); err != nil {
		g.logs.add("« ATTACK " + data)
		return
	}
	if lines, ok := reply["log"].([]any); ok {
		for _, l := range lines {
			g.logs.add("⚔ " + fmt.Sprint(l))
		}
	} else {
		g.logs.add(fmt.Sprintf("⚔ dégâts %v, PV de la cible %v, statut %v",
			reply["damage"], reply["target_hp"], reply["status"]))
	}
}

func (g *gui) applyStatus(data string) {
	var st map[string]any
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		g.logs.add("« STATUS " + data)
		return
	}
	hp, _ := st["hp"].(float64)
	maxHP, _ := st["max_hp"].(float64)
	text := fmt.Sprintf("PV %v / %v · %v", hp, maxHP, st["status"])
	// Champs en plus renvoyés par notre serveur (absents chez les autres groupes)
	for _, key := range []string{"level", "gold"} {
		if v, ok := st[key]; ok {
			text += fmt.Sprintf(" · %s %v", key, v)
		}
	}
	g.statusLabel.SetText(text)
	if maxHP > 0 {
		g.hpBar.SetValue(hp / maxHP)
	}
}

func (g *gui) applyQuests(data string) {
	var list []map[string]any
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		g.logs.add("« QUESTS " + data)
		return
	}
	g.quests = g.quests[:0]
	for _, q := range list {
		line := fmt.Sprintf("%v — %v", q["quest_id"], q["status"])
		if p, ok := q["progress"]; ok {
			line += fmt.Sprintf(" (%v)", p)
		}
		g.quests = append(g.quests, line)
	}
	refreshList(g.questsList)
}

// applyWho accepte le format RFC « players=N » et le format JSON de l'exemple du sujet.
func (g *gui) applyWho(data string) {
	if n, ok := counter(data, "players="); ok {
		g.serverCount = n
	} else {
		var who struct {
			Room   []string `json:"room"`
			Server int      `json:"server"`
		}
		if json.Unmarshal([]byte(data), &who) != nil {
			g.logs.add("« WHO " + data)
			return
		}
		g.serverCount = who.Server
	}
	g.updateCounters()
	g.logs.add(fmt.Sprintf("« %d joueur(s) connecté(s)", g.serverCount))
}

func (g *gui) applyGroup(sub, data string) {
	g.logs.add("✓ GROUP " + strings.TrimSpace(sub+" "+data))
	if id, ok := strings.CutPrefix(data, "group="); ok {
		g.groupLabel.SetText("Groupe : " + id)
	}
	if strings.EqualFold(sub, "LEAVE") {
		g.groupLabel.SetText("Groupe : aucun")
	}
}

func counter(s, prefix string) (int, bool) {
	v, ok := strings.CutPrefix(strings.TrimSpace(s), prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

func formatQuest(data string) string {
	var v map[string]any
	if json.Unmarshal([]byte(data), &v) != nil {
		return data
	}
	if desc, ok := v["description"]; ok {
		return fmt.Sprintf("%v — %v (récompense : %v)", v["quest_id"], desc, v["reward"])
	}
	return data
}
