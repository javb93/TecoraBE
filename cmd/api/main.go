package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tecora/internal/app"
	"tecora/internal/config"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
