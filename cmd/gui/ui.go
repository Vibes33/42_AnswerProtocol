package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const maxLines = 500 // lignes gardées par zone de texte

// chatScopes donne le scope CHAT de chaque onglet, dans l'ordre des onglets.
var (
	chatScopes = []string{"GLOBAL", "ROOM", "GROUP"}
	chatTitles = []string{"Global", "Room", "Group"}
)

// gui regroupe la fenêtre et tout l'état affiché. Il n'est modifié que depuis la goroutine
// de l'interface (callbacks des widgets et fonctions passées à fyne.Do).
type gui struct {
	app  fyne.App
	win  fyne.Window
	addr string
	cl   *client
	name string

	// Écran de connexion
	nameEntry *widget.Entry
	loginErr  *widget.Label

	// Salle
	roomTitle *widget.Label
	roomDesc  *widget.Label
	exits     *fyne.Container
	roomInfo  *container.Scroll
	dialogue  *widget.Label

	players, items, npcs, inventory, quests      []string
	playersList, itemsList, npcsList, invList    *widget.List
	questsList                                   *widget.List
	target                                       *widget.Entry
	roomCount, serverCount                       int
	counters, groupLabel, statusLabel, nameLabel *widget.Label
	hpBar                                        *widget.ProgressBar

	// Chat et logs
	chats     map[string]*textView
	chatTabs  *container.AppTabs
	chatEntry *widget.Entry
	logs      *textView

	inGame   bool
	quitting bool
}

// textView est une zone de texte en lecture seule qui défile automatiquement vers le bas.
type textView struct {
	label  *widget.Label
	scroll *container.Scroll
	lines  []string
}

func newTextView() *textView {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	return &textView{label: label, scroll: container.NewVScroll(label)}
}

func (t *textView) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > maxLines {
		t.lines = t.lines[len(t.lines)-maxLines:]
	}
	t.label.SetText(strings.Join(t.lines, "\n"))
	t.scroll.ScrollToBottom()
}

// newStringList affiche une liste de chaînes. Sélectionner une ligne la copie dans le champ
// « cible », utilisé ensuite par les boutons TAKE, DROP, TALK, ATTACK, QUEST et GROUP.
func (g *gui) newStringList(data *[]string) *widget.List {
	list := widget.NewList(
		func() int { return len(*data) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText((*data)[i]) },
	)
	list.OnSelected = func(i widget.ListItemID) {
		if i < len(*data) {
			g.target.SetText((*data)[i])
		}
	}
	return list
}

func refreshList(list *widget.List) {
	list.UnselectAll()
	list.Refresh()
}

func titled(title string, content fyne.CanvasObject) fyne.CanvasObject {
	header := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(header, nil, nil, nil, content)
}

func (g *gui) buildGame() fyne.CanvasObject {
	g.target = widget.NewEntry()
	g.target.SetPlaceHolder("Cible : objet, PNJ ou joueur (ID ou nom)")

	// Barre du haut : joueur, compteurs, groupe, points de vie
	g.nameLabel = widget.NewLabelWithStyle(g.name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	g.counters = widget.NewLabel("")
	g.groupLabel = widget.NewLabel("Groupe : aucun")
	g.statusLabel = widget.NewLabel("")
	g.hpBar = widget.NewProgressBar()
	g.hpBar.TextFormatter = func() string { return g.statusLabel.Text }
	g.updateCounters()
	top := container.NewBorder(nil, nil,
		container.NewHBox(g.nameLabel, widget.NewSeparator(), g.counters, widget.NewSeparator(), g.groupLabel),
		nil, g.hpBar)

	// Colonne de gauche : la salle courante
	g.roomTitle = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	g.roomTitle.SizeName = theme.SizeNameSubHeadingText
	g.roomDesc = widget.NewLabel("")
	g.roomDesc.Wrapping = fyne.TextWrapWord
	g.exits = container.NewGridWithColumns(3)
	g.dialogue = widget.NewLabel("")
	g.dialogue.Wrapping = fyne.TextWrapWord
	g.playersList = g.newStringList(&g.players)
	g.itemsList = g.newStringList(&g.items)
	g.npcsList = g.newStringList(&g.npcs)
	// La description est dans une zone qui défile : sinon, en passant à la ligne dans une
	// colonne étroite, elle écraserait les listes en dessous.
	g.roomInfo = container.NewVScroll(container.NewVBox(
		g.roomTitle, g.roomDesc, widget.NewLabel("Sorties (MOVE) :"), g.exits))
	room := container.NewVSplit(g.roomInfo, container.NewGridWithRows(3,
		titled("Joueurs ici", g.playersList),
		titled("Objets", g.itemsList),
		titled("PNJ et monstres", g.npcsList)))
	room.Offset = 0.4

	// Colonne de droite : inventaire et quêtes
	g.invList = g.newStringList(&g.inventory)
	g.questsList = g.newStringList(&g.quests)
	side := container.NewGridWithRows(3,
		titled("Inventaire", g.invList),
		titled("Quêtes", g.questsList),
		titled("Dialogue (TALK)", container.NewVScroll(g.dialogue)))

	// Centre : chat séparé par scope, puis logs du serveur
	g.chats = map[string]*textView{}
	var tabs []*container.TabItem
	for i, scope := range chatScopes {
		view := newTextView()
		g.chats[scope] = view
		tabs = append(tabs, container.NewTabItem(chatTitles[i], view.scroll))
	}
	g.chatTabs = container.NewAppTabs(tabs...)
	g.chatEntry = widget.NewEntry()
	g.chatEntry.SetPlaceHolder("Message pour l'onglet ouvert…")
	g.chatEntry.OnSubmitted = func(string) { g.sendChat() }
	chat := container.NewBorder(nil,
		container.NewBorder(nil, nil, nil, widget.NewButton("Envoyer", g.sendChat), g.chatEntry),
		nil, nil, g.chatTabs)
	center := container.NewVSplit(chat, titled("Logs du serveur", g.logs.scroll))
	center.Offset = 0.55

	right := container.NewHSplit(center, side)
	right.Offset = 0.66
	body := container.NewHSplit(room, right)
	body.Offset = 0.3

	return container.NewBorder(top, g.buildActions(), nil, nil, body)
}

// buildActions crée un bouton par commande du protocole. Les commandes qui ont besoin
// d'un argument le prennent dans le champ « cible ».
func (g *gui) buildActions() fyne.CanvasObject {
	withTarget := func(cmd string) *widget.Button {
		return widget.NewButton(cmd, func() { g.withTarget(cmd) })
	}
	simple := func(label, cmd string) *widget.Button {
		return widget.NewButton(label, func() { g.command(cmd) })
	}

	targeted := container.NewGridWithColumns(6,
		withTarget("TAKE"), withTarget("DROP"), withTarget("TALK"),
		withTarget("ATTACK"), withTarget("QUEST"), withTarget("MOVE"))
	general := container.NewGridWithColumns(6,
		simple("LOOK", "LOOK"), simple("INVENTORY", "INVENTORY"), simple("STATUS", "STATUS"),
		simple("QUESTS", "QUESTS"), simple("WHO", "WHO"),
		widget.NewButtonWithIcon("QUIT", theme.LogoutIcon(), func() { g.command("QUIT") }))
	groups := container.NewGridWithColumns(4,
		simple("GROUP CREATE", "GROUP CREATE"), withTarget("GROUP INVITE"),
		withTarget("GROUP JOIN"), simple("GROUP LEAVE", "GROUP LEAVE"))

	return container.NewVBox(widget.NewSeparator(), g.target, targeted, groups, general)
}

func (g *gui) updateCounters() {
	g.counters.SetText(fmt.Sprintf("Joueurs — salle : %d · serveur : %d", g.roomCount, g.serverCount))
}
