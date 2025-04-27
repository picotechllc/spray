package main

import (
	"context"
	"log"
)

func main() {
	flagCfg := parseFlags()
	cfg, err := loadConfig(flagCfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	logClient, err := createLoggingClient(ctx, cfg.projectID)
	if err != nil {
		log.Fatal(err)
	}
	defer logClient.Close()

	srv, err := DefaultServerSetup(ctx, cfg, logClient)
	if err != nil {
		log.Fatal(err)
	}

	if err := runServer(ctx, srv); err != nil {
		log.Fatal(err)
	}
}
