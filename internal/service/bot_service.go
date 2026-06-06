package service

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

type BotBuilderFunc = func(proxy string) (*bot.Bot, error)

func NewBotService(builder BotBuilderFunc, proxyService *ProxyService) *BotService {
	return &BotService{builder: builder, proxy: proxyService, restartChan: make(chan struct{})}
}

type BotService struct {
	builder BotBuilderFunc
	*bot.Bot
	proxy        *ProxyService
	botCtx       context.Context
	botCtxCancel context.CancelFunc
	restartChan  chan struct{}
}

func (s *BotService) Start(ctx context.Context) {
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)

	go s.startBot(ctx)

	for {
		_, ok := <-s.restartChan
		if !ok {
			log.Error().Msg("Restart channel closed, stopping bot...")
			return
		}
		log.Trace().Msg("Restarting bot...")
		s.botCtxCancel()
		s.startBot(ctx)
	}
}

var restartSF = singleflight.Group{}

func (s *BotService) Restart() {
	restartSF.Do("restart", func() (interface{}, error) {
		log.Trace().Msg("Sending restart signal...")
		s.restartChan <- struct{}{}
		return nil, nil
	})
}

func (s *BotService) Stop() {
	log.Trace().Msg("Stopping bot...")
	s.botCtxCancel()
	close(s.restartChan)
}

func (s *BotService) startBot(ctx context.Context) {
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		proxy, err := s.proxy.FirstAvailable()
		if err != nil {
			log.Error().Err(err).Msg("No available proxy, retrying in 10s...")
			time.Sleep(10 * time.Second)

			select {
			case <-ctx.Done():
				log.Error().Msg("Context cancelled, stopping bot...")
				return
			default:
			}
			continue
		}
		log.Debug().Str("proxy", proxy)

		s.Bot, err = s.builder(proxy)
		if err != nil {
			log.Error().Err(err).Msg("Failed to build bot, retrying in 10 second...")
			time.Sleep(10 * time.Second)
			continue
		}

		bot.WithErrorsHandler(func(err error) { go s.handleAPIError(err) })(s.Bot)

		break
	}

	s.Bot.Start(s.botCtx)
}

func (s *BotService) handleAPIError(err error) {
	if strings.Contains(err.Error(), "socks connect") || strings.Contains(err.Error(), "connection refused") {
		log.Error().Err(err).Msg("Proxy connection refused, restarting...")
		s.Restart()
	} else {
		log.Error().Err(err).Msg("Telegram API error")
	}
}
