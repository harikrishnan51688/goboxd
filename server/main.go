package main

import (
	"log"
	"net/http"
	"os"

	"goboxd/config"
	"goboxd/handler"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/app/config/languages.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded config: %d language(s)", len(cfg.Languages))

	h := handler.New(cfg)
	http.HandleFunc("/", h.Handle)

	addr := ":8000"
	log.Printf("Starting goboxd on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
