package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

func main() {
	bot := New(
		os.Getenv("TELEGRAM_ADMINS"),
		os.Getenv("TELEGRAM_CHATS"),
	)

	// default level is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if envLogLevel, ok := os.LookupEnv("LOG_LEVEL"); ok {
		zerologLevel, err := zerolog.ParseLevel(envLogLevel)
		if err == nil {
			zerolog.SetGlobalLevel(zerologLevel)
		}
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// init clients
	err := bot.InitMonoClients(os.Getenv("MONO_TOKENS"))
	if err != nil {
		log.Panic().Err(err)
	}

	go bot.TelegramStart(os.Getenv("TELEGRAM_TOKEN"))
	go bot.ProcessingStart()

	// run http server
	bot.WebhookStart()
}
