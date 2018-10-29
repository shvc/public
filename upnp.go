package main

import (
	"fmt"
	"io/ioutil"
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
var ServerURL = "http://192.168.56.1:23456/ping"

//var ServerURL = "http://www.aweg.cc:23456/ping"

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
	//flag.Parse()

	fmt.Printf("Time and Version  : %s @ %s\n", time.Now().Format("2006-01-02 15:04:05"), Version)
	// connect to router
	d, err := upnp.Discover()
	if err != nil {
		log.Println("Not find upnp router, ", err)
		fmt.Scanln()
		return
	}
	// record router's location
	loc := d.Location()
	fmt.Printf("Router locaion    : %s\n", loc)

	token := fmt.Sprintf("%d", time.Now().UnixNano())
	srv := &httptest.Server{
		Listener: newListener(),
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				r.ParseForm()
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(token))
				fmt.Printf("Real public IP    : %v\n", r.Form.Get("ip"))
			} else {
				log.Printf("Unexpected request: %s from %s\n", r.Method, r.RemoteAddr)
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
	//params.Set("ip", "Another server address")
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
	tokenBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("ReadAll response Body error: ", err)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	fmt.Printf("Server response   : %s\n", tokenBody)
	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		log.Println("upnp public IP err: ", err)
		clearPort(d, uint16(iport))
		fmt.Scanln()
		return
	}
	fmt.Printf("upnp public IP    : %s\n", ip)
	clearPort(d, uint16(iport))
	fmt.Scanln()

}
