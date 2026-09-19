package main

import (
	"bufio" // lecture ligne par ligne
	"flag"  // arguments en ligne de commande
	"fmt"   // affichage
	"log"   // erreurs fatales
	"net"   // TCP
	"os"    // stdin, exit
)

func main() {
	addr := flag.String("addr", "localhost:4242", "server address")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Printf("\r< %s\n> ", scanner.Text())
		}
		fmt.Println("\nconnection closed")
		os.Exit(0)
	}()

	stdin := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for stdin.Scan() {
		if _, err := conn.Write([]byte(stdin.Text() + "\n")); err != nil {
			log.Fatalf("write: %v", err)
		}
		fmt.Print("> ")
	}
}
