package main

import (
	"flag"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func main() {
	addr := flag.String("addr", "localhost:4242", "server address")
	flag.Parse()

	// Les logs existent dès le départ : le greeting du serveur arrive avant l'écran de jeu.
	g := &gui{app: app.NewWithID("fr.42.tap"), addr: *addr, logs: newTextView()}
	g.win = g.app.NewWindow("TAP — The Answer Protocol")
	g.win.Resize(fyne.NewSize(1200, 780))
	g.win.SetContent(g.buildLogin())
	g.win.SetOnClosed(func() {
		if g.cl != nil {
			g.cl.conn.Close()
		}
	})
	g.win.ShowAndRun()
}

func (g *gui) buildLogin() fyne.CanvasObject {
	g.nameEntry = widget.NewEntry()
	g.nameEntry.SetPlaceHolder("Ton pseudo")
	g.nameEntry.OnSubmitted = func(string) { g.login() }
	g.loginErr = widget.NewLabel("")
	g.loginErr.Wrapping = fyne.TextWrapWord

	form := container.NewVBox(
		widget.NewLabelWithStyle("The Answer Protocol", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Serveur : "+g.addr),
		g.nameEntry,
		widget.NewButton("Se connecter", g.login),
		g.loginErr,
	)
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(360, 260), form))
}

// login ouvre la connexion TCP au premier essai, puis envoie CONNECT. Si le pseudo est refusé,
// la connexion reste ouverte et le joueur peut réessayer avec un autre nom.
func (g *gui) login() {
	name := strings.TrimSpace(g.nameEntry.Text)
	if name == "" {
		g.loginErr.SetText("Choisis un pseudo.")
		return
	}
	g.name = name
	if g.cl != nil {
		g.send("CONNECT " + name)
		return
	}

	g.loginErr.SetText("Connexion à " + g.addr + "…")
	go func() { // net.Dial bloque : on le fait hors de la goroutine de l'interface
		cl, err := dial(g.addr)
		fyne.Do(func() {
			if err != nil {
				g.loginErr.SetText("Impossible de joindre le serveur : " + err.Error())
				return
			}
			g.cl = cl
			go cl.readLoop(g.onLine, g.onClosed)
			g.send("CONNECT " + name)
		})
	}()
}

// enterGame remplace l'écran de connexion par l'écran de jeu, puis charge l'état initial.
func (g *gui) enterGame() {
	g.inGame = true
	g.win.SetContent(g.buildGame())
	g.logs.add("Connecté en tant que " + g.name)
	g.refreshAll()
}

func (g *gui) onClosed() {
	if g.quitting {
		return
	}
	if !g.inGame {
		g.cl = nil // permet de réessayer depuis l'écran de connexion
		if g.loginErr.Text == "" || strings.HasPrefix(g.loginErr.Text, "Connexion à") {
			g.loginErr.SetText("Le serveur a fermé la connexion.")
		}
		return
	}
	info := dialog.NewInformation("Connexion perdue", "Le serveur a fermé la connexion.", g.win)
	info.SetOnClosed(g.app.Quit)
	info.Show()
}
