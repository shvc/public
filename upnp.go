package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/NebulousLabs/go-upnp"
)

func main() {
	fport := flag.Uint("port", 23456, "Listen port")
	flag.Parse()
	port := uint16(*fport)

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

	reqPath := "http://192.168.56.1"
	urlPath, err := url.Parse(reqPath)
	if err != nil {
		log.Fatal(err)
		return
	}
	params := url.Values{}
	params.Set("port", strconv.Itoa(int(port)))
	params.Set("token", "upnp-test")
	urlPath.RawQuery = params.Encode()
	resp, err := http.Get(urlPath.String())
	if err != nil {
		log.Fatal(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalln("ping server failed: ", resp.Status)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
		}
	})

	addrPort := fmt.Sprintf(":%d", port)
	fmt.Printf("server: %s", addrPort)
	err = http.ListenAndServe(addrPort, nil)
	if err != nil {
		log.Println("listen error: ", err)
	}

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
}
