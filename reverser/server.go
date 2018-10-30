package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func echoClientAddress(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	log.Printf("/ip %s %s %s\n", r.RemoteAddr, r.Method, r.RequestURI)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	remoteIP := r.RemoteAddr
	if index := strings.Index(r.RemoteAddr, ":"); index > 0 {
		remoteIP = r.RemoteAddr[:index]
	}
	data, _ := json.Marshal(map[string]interface{}{"ip": remoteIP})
	w.Write(data)
}

func process(w http.ResponseWriter, remoteIP string, port int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	respData := map[string]interface{}{"ip": remoteIP}

	urlPath, err := url.Parse(fmt.Sprintf("http://%s:%d/upnp", remoteIP, port))
	if err != nil {
		respData["result"] = "Server internal error"
		respBody, _ := json.Marshal(respData)
		w.Write(respBody)
		return
	}
	log.Println("request(Get) ->", urlPath.String())

	client := &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(urlPath.String())
	if err != nil {
		log.Printf("ping client(server) error : %s", err.Error())
		respData["result"] = err.Error()
		respBody, _ := json.Marshal(respData)
		w.Write(respBody)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("ping client(server) failed: %s", resp.Status)
		respData["result"] = resp.Status
		respBody, _ := json.Marshal(respData)
		w.Write(respBody)
	} else {
		defer resp.Body.Close()
		clientBody, _ := ioutil.ReadAll(resp.Body)
		log.Printf("ping client(server) success: %s", clientBody)
		respData["result"] = string(clientBody)
		respBody, _ := json.Marshal(respData)
		w.Write(respBody)
	}
}

func pingClient(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	log.Printf("/ping %s %s %s\n", r.RemoteAddr, r.Method, r.RequestURI)
	if r.Method == "GET" {
		r.ParseForm()
		port, err := strconv.Atoi(r.Form.Get("port"))
		if err != nil {
			http.Error(w, "Invalid parameter port", http.StatusBadRequest)
			return
		}

		remoteIP := ""
		if r.Form.Get("ip") != "" {
			remoteIP = r.Form.Get("ip")
		} else {
			if index := strings.Index(r.RemoteAddr, ":"); index > 0 {
				remoteIP = r.RemoteAddr[:index]
			} else {
				log.Println("No valid remoteIP to use")
				http.Error(w, "server Address error", http.StatusInternalServerError)
				return
			}
		}
		go process(w, remoteIP, port)
	} else {
		http.Error(w, "Bad request", http.StatusBadRequest)
	}
}

func main() {
	fport := flag.Uint("port", 23456, "Listen port")
	flag.Parse()
	addrPort := fmt.Sprintf(":%d", *fport)
	logfile := "/tmp/upnpchecker.log"
	fd, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("open logfile %s, error: %s", logfile, err)
		os.Exit(1)
	}
	logPrefix := fmt.Sprintf("%d ", os.Getpid())
	log.SetPrefix(logPrefix)
	log.SetOutput(fd)

	http.HandleFunc("/ip", echoClientAddress)
	http.HandleFunc("/ping", pingClient)

	//http.Handle("/pkgs/", http.StripPrefix("/pkgs/", http.FileServer(http.Dir(filepath.Join("tmp"), "pkgs")))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<a href=\"/ip\">Get your public ip</a>\n"))
		w.Write([]byte("<br>\n"))
		w.Write([]byte("<a href=\"/pkgs\">Get test app</a>\n"))
		w.Write([]byte("<br>\n"))
	})

	log.Printf("listen: %s\n", addrPort)
	err = http.ListenAndServe(addrPort, nil)
	if err != nil {
		log.Println("listen error: ", err)
	}
}
