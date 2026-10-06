package main

import (
	"bufio" // line-by-line reading
	"flag"  // command-line arguments
	"fmt"   // output
	"log"   // fatal errors
	"net"   // TCP
	"os"    // stdin, exit
)

const serverFull = "ERR 900 SERVER_FULL"

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
			line := scanner.Text()
			if line == serverFull {
				fmt.Println("\rServer full: the maximum number of players has been reached")
				os.Exit(1)
			}
			fmt.Printf("\r< %s\n> ", line)
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
