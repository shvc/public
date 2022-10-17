package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gobike/envflag"
)

var (
	addr             = ":80"
	readTimeout uint = 30
	idleTimeout uint = 20
	stopTimeout uint = 30
	hostname         = "unkonwn"
)

func router() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK,
			gin.H{
				"code":    http.StatusOK,
				"message": "pong",
				"server":  hostname,
			})
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

func main() {
	flag.StringVar(&addr, "addr", addr, "serve address")
	envflag.Parse()

	if hn, err := os.Hostname(); err == nil {
		hostname = hn
	}

	server := http.Server{
		Addr:        addr,
		Handler:     router(),
		ReadTimeout: time.Duration(readTimeout) * time.Minute,
		IdleTimeout: time.Duration(idleTimeout) * time.Minute,
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
