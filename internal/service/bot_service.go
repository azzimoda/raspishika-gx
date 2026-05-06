package service

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
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
			return
		}
		s.Stop()
		s.startBot(s.botCtx)
	}
}

func (s *BotService) Restart() { s.restartChan <- struct{}{} }

func (s *BotService) Stop() {
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
			continue
		}
		log.Debug().Str("proxy", proxy)

		s.Bot, err = s.builder(proxy)
		if err != nil {
			log.Error().Err(err).Msg("Failed to build bot, retrying in 10 second...")
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}

	s.Bot.Start(ctx)
}
