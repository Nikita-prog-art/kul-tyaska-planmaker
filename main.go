// Команда kul-tyaska — планировщик перерождений для культяшки.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kultyaska/internal/config"
	"kultyaska/internal/store"
	"kultyaska/internal/web"
)

func main() {
	showVersion := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()
	if *showVersion {
		log.Printf("kul-tyaska %s", web.Version)
		return
	}

	cfg := config.Load()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("хранилище: %v", err)
	}
	defer st.Close()

	srv, err := web.New(cfg, st)
	if err != nil {
		log.Fatalf("шаблоны: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("kul-tyaska %s: слушаю %s, база %s", web.Version, cfg.Addr, cfg.DBPath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("сервер: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("останов: %v", err)
	}
	log.Println("пока")
}
