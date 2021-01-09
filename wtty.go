package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/term"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:80", "address")
	cmd := flag.String("cmd", "bash", "command")
	flag.Parse()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	if !strings.Contains(*addr, ":") || strings.HasSuffix(*addr, ":") {
		*addr = fmt.Sprintf("%s:80", strings.Trim(*addr, ":"))
	}
	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		panic(err)
	}
	body := fmt.Sprintf("GET /exec/%s HTTP/1.1\nHost:%s\n\n", *cmd, *addr)
	io.WriteString(conn, body)
	go io.Copy(conn, os.Stdin)
	io.Copy(os.Stdout, conn)
}
