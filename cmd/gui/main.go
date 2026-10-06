// Command gui runs the TAP web client on its own, without a game server: useful to play on
// another group's server (-addr). tap-server already serves this same client on port 8042.
// The interface and the relay live in internal/web.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"tap/internal/web"
)

func main() {
	addr := flag.String("addr", "localhost:4242", "TAP server address")
	listen := flag.String("listen", "127.0.0.1:8043", "address of the web interface")
	lan := flag.Bool("lan", false, "listen on every network interface so other computers can join")
	noOpen := flag.Bool("no-open", false, "do not open the browser automatically")
	flag.Parse()

	if *lan {
		_, port, err := net.SplitHostPort(*listen)
		if err != nil {
			log.Fatal(err)
		}
		*listen = net.JoinHostPort("0.0.0.0", port)
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	local := "http://127.0.0.1:" + port
	log.Printf("TAP web client on %s (game server %s)", local, *addr)
	if *lan {
		for _, ip := range web.LANIPs() {
			log.Printf("other computers on the network: http://%s:%s", ip, port)
		}
	}
	if !*noOpen {
		openBrowser(local)
	}
	log.Fatal(http.Serve(ln, web.Handler(*addr)))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("open %s manually (%v)", url, err)
	}
}
