package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kianPk/gateproxy/internal/config"
	"github.com/kianPk/gateproxy/internal/metrics"
	"github.com/kianPk/gateproxy/internal/proxy"
)

//go:embed web/*
var webFS embed.FS

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "gateproxy ", log.LstdFlags|log.Lmsgprefix)
	reg := metrics.New()
	gw := proxy.New(cfg, reg, logger)
	api := gw.Handler()

	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		logger.Fatal(err)
	}
	static := http.FileServer(http.FS(webRoot))

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" || p == "/index.html" || p == "/app.js" || p == "/style.css" || strings.HasPrefix(p, "/assets/") {
			static.ServeHTTP(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:         cfg.Listen,
		Handler:      root,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	go func() {
		logger.Printf("listening on %s · %d routes · JWT+rate-limit gateway", cfg.Listen, len(cfg.Routes))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	logger.Println("shutdown complete")
}
