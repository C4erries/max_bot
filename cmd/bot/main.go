package main

import (
	"context"
	"fmt"
	"os"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
)

var token = "f9LHodD0cOJTo2jim5VSB3XegdV5eYOa0orYqub31xNn68MMqVnwDM6fLsTuQXINU4uCaQsF4nh6883oZEB9"

func main() {
	api, err := maxbot.New(token)
	if err != nil {
		fmt.Printf("Error creating Max Bot API client: %v", err)
		os.Exit(1)
	}
	ctx := context.Background()
	info, err := api.Bots.GetBot(ctx)
	fmt.Printf("Get me: %#v %#v", info, err)
}
