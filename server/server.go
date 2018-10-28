package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func echoClientAddress(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	log.Printf("ping : %s %s %s\n", r.RemoteAddr, r.Method, r.RequestURI)
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
	log.Printf("ping : %s %s %s\n", r.RemoteAddr, r.Method, r.RequestURI)
	if r.Method == "GET" {
		r.ParseForm()
		if r.Form.Get("port") == "" || r.Form.Get("token") == "" {
			http.Error(w, "No parameter port and token", http.StatusBadRequest)
			return
		}
		port, err := strconv.Atoi(r.Form.Get("port"))
		if err != nil {
			http.Error(w, "Invalid parameters", http.StatusBadRequest)
			return
		}
		token := r.Form.Get("token")

		remoteIP := r.RemoteAddr
		if index := strings.Index(r.RemoteAddr, ":"); index > 0 {
			remoteIP = r.RemoteAddr[:index]
		}

		urlPath, err := url.Parse(fmt.Sprintf("http://%s:%d", remoteIP, port))
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		params := url.Values{}
		params.Set("port", r.Form["port"][0])
		params.Set("token", token)
		urlPath.RawQuery = params.Encode()
		log.Println("request :", urlPath.String())
		resp, err := http.Get(urlPath.String())
		if err != nil {
			log.Println(err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			log.Println("ping server failed: ", resp.Status)
		}
	} else {
		http.Error(w, "Bad request", http.StatusBadRequest)
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

	fmt.Printf("server: %s\n", addrPort)
	err := http.ListenAndServe(addrPort, nil)
	if err != nil {
		log.Println("listen error: ", err)
	}
}
