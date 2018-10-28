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
)

func newListener() net.Listener {
	l, err := net.Listen("tcp", "0.0.0.0:22334")
	if err != nil {
		fmt.Println("Listen 0.0.0.0 failed: ", err)
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
	}
	return l
}

func main() {
	fport := flag.Uint("port", 22334, "Listen port")
	flag.Parse()
	port := uint16(*fport)

	/*
		// connect to router
		d, err := upnp.Discover()
		if err != nil {
			log.Fatal(err)
		}

		// forward a port
		err = d.Forward(port, "myshare-upnp-checker")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("map port:%d success\n", port)

		// un-forward a port
		err = d.Clear(port)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("unmap port:%d success\n", port)
	*/
	token := fmt.Sprintf("%d", time.Now().UnixNano())

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test_requestServer"))
		if r.Method == "GET" {
			r.ParseForm()
			if token != r.Form.Get("token") {
				log.Printf("Expected request to have 'token=%s', got: '%s'", token, r.Form.Get("token"))
			}
			fmt.Printf("response: %v\n", r.URL)
		} else {
			log.Printf("Expected 'GET' request, got '%s'", r.Method)

		}

	}))
	srv.Listener = newListener()
	srv.Start()
	fmt.Printf("test server ==> %#v\n", srv.URL)
	portIndex := strings.LastIndex(srv.URL, ":")
	fmt.Printf("port: %s\n", srv.URL[portIndex:])

	reqPath := "http://192.168.56.1:23456/ping"
	//reqPath := srv.URL
	urlPath, err := url.Parse(reqPath)
	if err != nil {
		log.Fatal(err)
		return
	}
	params := url.Values{}
	params.Set("port", strconv.Itoa(int(port)))
	params.Set("token", token)
	urlPath.RawQuery = params.Encode()
	fmt.Printf("request Get %s\n", urlPath.String())
	resp, err := http.Get(urlPath.String())
	if err != nil {
		log.Fatal(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalln("ping server failed: ", resp.Status)
	}

	time.Sleep(5000)

	/*
		addrPort := fmt.Sprintf(":%d", port)
		fmt.Printf("listenAdnServe: %s\n", addrPort)
		err = http.ListenAndServe(addrPort, nil)
		if err != nil {
			log.Println("listen error: ", err)
		}
	*/

	/*
		// discover external IP
		ip, err := d.ExternalIP()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Your external IP is:", ip)

		// record router's location
		loc := d.Location()
		fmt.Println("router upnp info: ", loc)

		// connect to router directly
		d, err = upnp.Load(loc)
		if err != nil {
			log.Fatal(err)
		}
	*/
}
