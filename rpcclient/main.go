package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
)

var (
	addr  = "127.0.0.1:4444"
	proto = "http"
)

type Item struct {
	Key   string
	Value int
}

func DialHTTPPath(network, address, path string) (*rpc.Client, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.StatusCode == 200 {
		return jsonrpc.NewClient(conn), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  network + " " + address,
		Addr: nil,
		Err:  err,
	}
}

func main() {
	flag.StringVar(&addr, "addr", addr, "serv address")
	flag.StringVar(&proto, "proto", proto, "proto")
	flag.Parse()

	var reply Item
	var db []Item
	var err error
	var client *rpc.Client
	switch strings.ToUpper(proto) {
	case "HTTP":
		client, err = DialHTTPPath("tcp", addr, rpc.DefaultRPCPath)
	default:
		client, err = jsonrpc.Dial(proto, addr)
	}

	if err != nil {
		log.Fatal("Connection error: ", err)
	}

	a := Item{"a", 1}
	b := Item{"b", 2}
	c := Item{"c", 3}

	client.Call("RPC.AddItem", a, &reply)
	client.Call("RPC.AddItem", b, &reply)
	client.Call("RPC.AddItem", c, &reply)

	client.Call("RPC.GetDB", "", &db)
	fmt.Println("GetDB: ", db)

	client.Call("RPC.EditItem", Item{"a", 11}, &reply)

	client.Call("RPC.DeleteItem", c, &reply)
	client.Call("RPC.GetDB", "", &db)
	fmt.Println("GetDB(after delete): ", db)

	client.Call("RPC.GetByName", "a", &reply)
	fmt.Println("GetByName(a): ", reply)

}
