package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/gin-gonic/gin"
	"github.com/gobike/envflag"
	"github.com/wolfogre/go-pprof-practice/animal"
)

var (
	addr                   = ":80"
	readHeaderTimeout uint = 10
	idleTimeout       uint = 600
	stopTimeout       uint = 30
	cpuNum            uint = 1
	debug                  = false
	hostname               = "unkonwn"
)

func router() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.GET("/ping", func(c *gin.Context) {
		c.SetCookie("cookie-name", "cookie-value", 600, "/", "", true, true)
		ck, _ := c.Cookie("cookie-name")
		c.JSON(http.StatusOK,
			gin.H{
				"code":    http.StatusOK,
				"message": "pong",
				"server":  hostname,
				"cookie":  ck,
			},
		)
	})

	e.GET("/memstats", func(c *gin.Context) {
		m := runtime.MemStats{}
		runtime.ReadMemStats(&m)
		c.JSON(http.StatusOK, m)
	})

	e.GET("/user/:name", func(c *gin.Context) {
		name := c.Param("name")
		c.String(http.StatusOK, "Hello %s", name)
	})

	e.GET("/user/:name/*action", func(c *gin.Context) {
		name := c.Param("name")
		action := c.Param("action")
		message := name + " is " + action
		c.String(http.StatusOK, message)
	})

	e.POST("/user/:name/*action", func(c *gin.Context) {
		b := c.FullPath() == "/user/:name/*action"
		c.String(http.StatusOK, "%t", b)
	})

	e.GET("/user/groups", func(c *gin.Context) {
		c.String(http.StatusOK, "The available groups are [...]")
	})

	return e
}

func debugAndPprof(cpuNum int) {
	runtime.GOMAXPROCS(int(cpuNum))
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	go func() {
		log.Println("pprof", http.ListenAndServe(":9009", nil))
	}()

	for {
		for _, v := range animal.AllAnimals {
			v.Live()
		}
		time.Sleep(1 * time.Second)
	}
}

func main() {
	flag.StringVar(&addr, "addr", addr, "serve address")
	flag.UintVar(&readHeaderTimeout, "read-header-timeout", readHeaderTimeout, "http read header timeout in second")
	flag.UintVar(&idleTimeout, "idle-timeout", idleTimeout, "http idle timeout in second")
	flag.UintVar(&cpuNum, "cpu", cpuNum, "cpu number")
	flag.BoolVar(&debug, "debug", debug, "debug(pprof)")
	envflag.Parse()

	if hn, err := os.Hostname(); err == nil {
		hostname = hn
	}

	if debug {
		go debugAndPprof(int(cpuNum))
	}

	server := http.Server{
		Addr:              addr,
		Handler:           router(),
		ReadHeaderTimeout: time.Duration(readHeaderTimeout) * time.Second,
		IdleTimeout:       time.Duration(idleTimeout) * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				log.Println("stopping server")
			} else {
				log.Println("ListenAndServe failed", err.Error())

			}
		} else {
			log.Println("sever ListenAndServe exception")
		}
	}()

	<-done
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(stopTimeout)*time.Minute)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Println("server shutdown error", err)
		return
	}
	log.Println("server gracefully stopped")
}
