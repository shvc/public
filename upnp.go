package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	upnp "github.com/NebulousLabs/go-upnp"
)

// Version to record build bersion
var Version = "1.0.3"

// ServerURL default Server URL
//var ServerURL = "http://192.168.56.1:23456/ping"
var ServerURL = "http://www.aweg.cc:23456/ping"

func newListener() net.Listener {
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		fmt.Println("Listen 0.0.0.0 failed: ", err)
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
	}
	return l
}

func clearPort(d *upnp.IGD, port uint16) {
	// un-forward a port
	err := d.Clear(port)
	if err != nil {
		log.Printf("Failed unmap port : %d , %s ", port, err.Error())
		fmt.Scanln()
	} else {
		fmt.Printf("Success unmap port: %d\n", port)
	}
}

func main() {
	successResult := false
	waitSec := flag.Duration("wait", 5, "wait seconds")
	flag.Parse()

	// connect to router
	d, err := upnp.Discover()
	if err != nil {
		log.Fatal("Not find upnp router, ", err)
	}
	fmt.Printf("Time and Version  : %s @ %s\n", time.Now().Format("2006-01-02 15:04:05"), Version)
	// record router's location
	loc := d.Location()
	fmt.Printf("Router locaion    : %s\n", loc)

	token := fmt.Sprintf("%d", time.Now().UnixNano())
	srv := &httptest.Server{
		Listener: newListener(),
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test server"))
			if r.Method == "GET" {
				r.ParseForm()
				if token != r.Form.Get("token") {
					log.Printf("Unexpected token  : %s\n", r.Form.Get("token"))
				}
				fmt.Printf("Real public IP    : %v\n", r.Form.Get("ip"))
				successResult = true
			} else {
				log.Printf("Unexpected request:%s\n", r.Method)
			}
		})},
	}
	srv.Start()

	portIndex := strings.LastIndex(srv.URL, ":")
	if portIndex < 0 || portIndex >= len(srv.URL) {
		log.Printf("Can not get port from %s", srv.URL)
		fmt.Scanln()
		return
	}
	lport := srv.URL[portIndex+1:]
	iport, _ := strconv.Atoi(lport)

	// upnp forward a port
	err = d.Forward(uint16(iport), "myshare-check")
	if err != nil {
		log.Printf("Failed map port   : %d , %s ", iport, err.Error())
		fmt.Scanln()
		return
	}
	fmt.Printf("Success map port  : %d\n", iport)

	urlPath, err := url.Parse(ServerURL)
	if err != nil {
		log.Println("Parse URL error  : ", err)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	params := url.Values{}
	params.Set("port", lport)
	params.Set("token", token)
	urlPath.RawQuery = params.Encode()

	resp, err := http.Get(urlPath.String())
	if err != nil {
		log.Println("Request URL error : ", err)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("Server response failed ", resp.Status)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		log.Println("upnp public IP err: ", err)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	fmt.Printf("upnp public IP    : %s\n", ip)

	time.Sleep(*waitSec * time.Second)
	clearPort(d, uint16(iport))

	if successResult {
		fmt.Println("Success!")
	} else {
		fmt.Println("Failed!")
	}
	fmt.Scanln()

}
