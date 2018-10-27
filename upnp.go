package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/NebulousLabs/go-upnp"
)

func main() {
	fport := flag.Uint("port", 9902, "Listen port")
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
