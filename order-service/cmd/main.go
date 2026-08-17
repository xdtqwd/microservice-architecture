package main

import (
	"context"
	"log"
	"order-service/internal/app"
)

func main() {
	ctx := context.Background()

	app, err := app.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(app.Run())
}
