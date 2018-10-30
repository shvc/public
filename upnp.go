package main

import (
	"encoding/json"
	"fmt"
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

func confirmExit() {
	fmt.Printf("Press Enter to exit")
	fmt.Scanln()
}

func clearPort(d *upnp.IGD, port uint16) {
	// un-forward a port
	err := d.Clear(port)
	if err != nil {
		fmt.Printf("Failed unmap port : %d , %s \n\n", port, err.Error())
		confirmExit()
	} else {
		fmt.Printf("Success unmap port: %d\n\n", port)
	}
}

func main() {
	//flag.Parse()

	fmt.Printf("Time and Version  : %s @ %s\n", time.Now().Format("2006-01-02 15:04:05"), Version)
	// connect to router
	d, err := upnp.Discover()
	if err != nil {
		fmt.Printf("Not find upnp Router: %s\n\n", err.Error())
		confirmExit()
		return
	}
	// record router's location
	loc := d.Location()
	fmt.Printf("Router locaion    : %s\n", loc)

	srv := &httptest.Server{
		Listener: newListener(),
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				if r.URL.EscapedPath() != "/upnp" {
					w.Write([]byte("Invalid URL !"))
				} else {
					w.Write([]byte("Success !"))
				}
			} else {
				fmt.Printf("Unexpected request: %s from %s\n", r.Method, r.RemoteAddr)
			}
		})},
	}
	srv.Start()

	portIndex := strings.LastIndex(srv.URL, ":")
	if portIndex < 0 || portIndex >= len(srv.URL) {
		fmt.Printf("Can not get port from %s \n\n", srv.URL)
		confirmExit()
		return
	}
	lport := srv.URL[portIndex+1:]
	iport, _ := strconv.Atoi(lport)

	// upnp forward a port
	err = d.Forward(uint16(iport), "myshare-check")
	if err != nil {
		fmt.Printf("Failed map port   : %d , %s \n\n", iport, err.Error())
		confirmExit()
		return
	}
	fmt.Printf("Success map port  : %d\n", iport)

	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		fmt.Println("upnp public IP err:", err)
	} else {
		fmt.Println("upnp public IP    :", ip)
	}

	urlPath, err := url.Parse(ServerURL)
	if err != nil {
		fmt.Printf("Parse URL error  : %s\n\n", err.Error())
		clearPort(d, uint16(iport))
		confirmExit()
		return
	}
	params := url.Values{}
	params.Set("port", lport)
	//params.Set("ip", "Another server address")
	urlPath.RawQuery = params.Encode()

	resp, err := http.Get(urlPath.String())
	if err != nil {
		fmt.Println("Request URL error : ", err)
		clearPort(d, uint16(iport))
		confirmExit()
		return
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Server response   :", resp.Status)
		clearPort(d, uint16(iport))
		confirmExit()
		return
	}
	respData := map[string]interface{}{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		fmt.Println("Server response   :", err)
		return
	}
	fmt.Printf("Real public IP    : %s\n", respData["ip"])
	fmt.Printf("Test result       : %s\n", respData["result"])

	clearPort(d, uint16(iport))
	confirmExit()

}
