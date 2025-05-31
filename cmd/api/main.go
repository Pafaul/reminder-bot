package main

import (
	"context"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/signal"
	"pafaul/reminder"
)

// Send any text message to the bot after the bot has been started

func main() {
	tokenBot := os.Getenv("TELEGRAM_BOT_TOKEN")

	bot := reminder.NewBot(tokenBot)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer (func() {
		cancel()
	})()

	bot.Start(ctx)
}
