package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:80", "address")
	cmd := flag.String("cmd", "bash", "command")
	flag.Parse()
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
