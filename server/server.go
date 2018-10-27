package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func echoClientAddress(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	remoteIP := r.RemoteAddr
	if index := strings.Index(r.RemoteAddr, ":"); index > 0 {
		remoteIP = r.RemoteAddr[:index]
	}
	data, _ := json.Marshal(map[string]interface{}{"ip": remoteIP})
	w.Write(data)
}

func pingClient(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.Method == "POST" {
		w.Header().Set("Content-Type", "application/json")

		data := map[string]interface{}{}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			log.Println("decode json error: ", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("failed"))
			return
		}
		remoteIP := r.RemoteAddr
		if index := strings.Index(r.RemoteAddr, ":"); index > 0 {
			remoteIP = r.RemoteAddr[:index]
		}
		clientURL := fmt.Sprintf("%s:%s", remoteIP, data["port"])
		http.Post(clientURL, "application/json", r.Body)
	}
}

func main() {
	fport := flag.Uint("port", 23456, "Listen port")
	flag.Parse()
	addrPort := fmt.Sprintf(":%d", *fport)

	http.HandleFunc("/ip", echoClientAddress)
	http.HandleFunc("/ping", pingClient)

	//http.Handle("/pkgs/", http.StripPrefix("/pkgs/", http.FileServer(http.Dir(*filedir))))

	http.HandleFunc("/upnp", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upnp"))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<a href=\"/ip\">Get public ip</a>\n"))
		w.Write([]byte("<br>\n"))
		w.Write([]byte("<a href=\"/upnp\">Test upnp</a>\n"))
	})

	fmt.Printf("server: %s", addrPort)
	err := http.ListenAndServe(addrPort, nil)
	if err != nil {
		log.Println("listen error: ", err)
	}
}
