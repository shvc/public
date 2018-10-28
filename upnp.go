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
		log.Printf("unmap upnp port[%d] error: %s ", port, err.Error())
		fmt.Scanln()
	} else {
		fmt.Printf("unmap port[%d] success\n", port)
	}
}

func main() {
	checkResult := false
	serverURL := flag.String("port", "http://192.168.56.1:23456/ping", "Server URL")
	flag.Parse()

	// connect to router
	d, err := upnp.Discover()
	if err != nil {
		log.Fatal("Not find upnp router, ", err)
	}
	// record router's location
	loc := d.Location()
	log.Println("Router locaion: ", loc)

	token := fmt.Sprintf("%d", time.Now().UnixNano())
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test server"))
		if r.Method == "GET" {
			r.ParseForm()
			if token != r.Form.Get("token") {
				log.Printf("Failed, expected 'token=%s', got: '%s'", token, r.Form.Get("token"))
			}
			fmt.Printf("Your public IP: %v\n", r.Form.Get("ip"))
			checkResult = true
		} else {
			log.Printf("Failed, expected 'GET' request, got '%s'", r.Method)
		}
	}))
	srv.Listener = newListener()
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
		log.Printf("upnp map port[%d] failed %s ", iport, err.Error())
		fmt.Scanln()
		return
	}
	fmt.Printf("upnp map port[%d] success\n", iport)
	defer clearPort(d, uint16(iport))

	urlPath, err := url.Parse(*serverURL)
	if err != nil {
		log.Println("Parse URL error: ", err)
		fmt.Scanln()
		return
	}
	params := url.Values{}
	params.Set("port", lport)
	params.Set("token", token)
	urlPath.RawQuery = params.Encode()

	resp, err := http.Get(urlPath.String())
	if err != nil {
		log.Println("request URL error: ", err)
		fmt.Scanln()
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("server response failed ", resp.Status)
		fmt.Scanln()
		return
	}

	time.Sleep(2000)

	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		log.Println("upnp externalIP error: ", err)
		fmt.Scanln()
		return
	}
	fmt.Println("upnp public IP:", ip)
	if checkResult {
		fmt.Println("Success!!")
	} else {
		fmt.Println("Failed!")
	}
	fmt.Scanln()

}
