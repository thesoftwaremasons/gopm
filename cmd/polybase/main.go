package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/thesoftwaremasons/gopm/internal/polybase/api"
)

func main() {
	port    := flag.Int("port", 7777, "HTTP listen port")
	dataDir := flag.String("data", defaultDataDir(), "data directory for metadata and master key")
	flag.Parse()

	srv, err := api.NewServer(*port, *dataDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Polybase running at http://localhost:%d\n", *port)
	log.Fatal(srv.ListenAndServe())
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".polybase")
}
