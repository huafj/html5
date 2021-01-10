package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
)

type simpleTransport struct {
	conn net.Conn
}

func (tr *simpleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{}
	host := req.URL.Hostname()
	port := req.URL.Port()
	addr := net.JoinHostPort(host, port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	tr.conn = conn
	body := fmt.Sprintf("%s %s %s\nHost:%s\n\n", req.Method, req.URL.Path, req.Proto, addr)
	if _, err = io.WriteString(conn, body); err != nil {
		resp.StatusCode = http.StatusInternalServerError
	} else {
		resp.StatusCode = http.StatusOK
	}
	return resp, nil
}

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
	tr := simpleTransport{}
	client := http.Client{
		Transport: &tr,
	}
	url := fmt.Sprintf("http://%s/exec/%s", *addr, *cmd)
	if _, err := client.Get(url); err != nil {
		panic(err)
	}
	conn := tr.conn
	go io.Copy(conn, os.Stdin)
	io.Copy(os.Stdout, conn)
}
