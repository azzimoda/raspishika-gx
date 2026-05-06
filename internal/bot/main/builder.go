package mainbot

import (
	"context"
	_ "embed"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
	"github.com/azzimoda/raspishika-gx/pkg/reporter"
)

//go:embed commands.yaml
var myCommandsBytes []byte

func New(s *service.Services, p string, reporter reporter.Reporter) (*bot.Bot, error) {
	h := newHandler(s, reporter)

	httpClient, err := proxyutil.NewHTTPProxyClient(p)
	if err != nil {
		return nil, err
	}

	opts := []bot.Option{
		bot.WithHTTPClient(10*time.Second, httpClient),
		// bot.WithCheckInitTimeout(30 * time.Second),
		bot.WithMiddlewares(
			h.ignoreOldMessage,
			h.ignoreInaccessibleMessageCQ,
			h.syncCQSingleFlight,
			h.ensureChat,
			h.logHandler,
		),
		bot.WithDefaultHandler(h.handleDefault),
	}
	if viper.GetString(config.KeyLogLevel) == "trace" {
		opts = append(opts, bot.WithDebug())
	}
	b, err := bot.New(viper.GetString(config.KeyBotToken), opts...)
	if err != nil {
		return nil, err
	}

	h.registerHandlers(b)

	myCommands, err := botutil.ParseBotCommands(myCommandsBytes)
	if err != nil {
		panic(err)
	}
	if ok, err := b.SetMyCommands(
		context.Background(),
		&bot.SetMyCommandsParams{Commands: myCommands},
	); err != nil || !ok {
		log.Error().Err(err).Any("myCommands", myCommands).Msg("Failed to bot commands")
	} else {
		log.Debug().Msg("My commands set")
	}

	return b, nil
}
