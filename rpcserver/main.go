package main

import (
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
	addr  = ":4444"
	proto = "http"
)

type Item struct {
	Key   string
	Value int
}

type RPC struct{}

var store []Item

func (a *RPC) GetDB(empty string, reply *[]Item) error {
	log.Println("getDb")
	if reply != nil {
		*reply = store
	}
	return nil
}

func (a *RPC) GetByName(title string, reply *Item) error {
	log.Println("GetByName", title)
	var getItem Item
	for _, val := range store {
		if val.Key == title {
			getItem = val
		}
	}

	if reply != nil {
		*reply = getItem
	}
	return nil
}

func (a *RPC) AddItem(item Item, reply *Item) error {
	log.Println("AddItem", item.Key)
	store = append(store, item)
	if reply != nil {
		*reply = item
	}
	return nil
}

func (a *RPC) EditItem(item Item, reply *Item) error {
	log.Println("EditItem", item.Key)
	var changed Item
	for idx, val := range store {
		if val.Key == item.Key {
			store[idx] = Item{item.Key, item.Value}
			changed = store[idx]
		}
	}
	if reply != nil {
		*reply = changed
	}
	return nil
}

func (a *RPC) DeleteItem(item Item, reply *Item) error {
	log.Println("DeleteItem", item.Key)
	var del Item
	for idx, val := range store {
		if val.Key == item.Key && val.Value == item.Value {
			store = append(store[:idx], store[idx+1:]...)
			del = item
			break
		}
	}

	if reply != nil {
		*reply = del
	}
	return nil
}

func initServer() *RPC {
	r := &RPC{}
	fmt.Println("initial database: ", store)
	a := Item{"x", 7}
	b := Item{"y", 8}
	c := Item{"z", 9}

	r.AddItem(a, nil)
	r.AddItem(b, nil)
	r.AddItem(c, nil)
	fmt.Println("second database: ", store)

	r.DeleteItem(b, nil)
	fmt.Println("third database: ", store)

	r.EditItem(Item{"z", 99}, nil)
	fmt.Println("fourth database: ", store)

	var x, y Item
	r.GetByName("x", &x)
	r.GetByName("y", &y)
	fmt.Println(x, y)
	return r
}

func main() {
	flag.StringVar(&addr, "addr", addr, "serv address")
	flag.StringVar(&proto, "proto", proto, "proto")
	flag.Parse()

	app := initServer()
	err := rpc.RegisterName("myrpc", app)
	if err != nil {
		log.Fatal("error registering RPC", err)
	}

	switch strings.ToUpper(proto) {
	case "HTTP":
		http.HandleFunc("/jsonrpc", func(w http.ResponseWriter, r *http.Request) {
			var conn io.ReadWriteCloser = struct {
				io.Writer
				io.ReadCloser
			}{
				ReadCloser: r.Body,
				Writer:     w,
			}

			rpc.ServeRequest(jsonrpc.NewServerCodec(conn))
		})
		err = http.ListenAndServe(addr, nil)
		if err != nil {
			log.Fatal("rpc http serving error:", err)
		}
	default:
		listener, err := net.Listen(proto, addr)
		if err != nil {
			log.Fatal("Listen error:", err)
		}

		// rpc.Accept(listener)
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Accept error:", err)
				continue
			}
			go jsonrpc.ServeConn(conn)
		}

	}

}
