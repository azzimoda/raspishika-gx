package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

type BotBuilderFunc = func(proxy string, onActivity func()) (*bot.Bot, error)

func NewBotService(builder BotBuilderFunc, proxyService *ProxyService) *BotService {
	return &BotService{builder: builder, proxy: proxyService, restartChan: make(chan struct{})}
}

const (
	proxyWatchdogPeriod = 30 * time.Second
	proxyStallTimeout   = 3 * time.Minute
)

type BotService struct {
	superCtx context.Context
	username string
	builder  BotBuilderFunc
	*bot.Bot
	proxy        *ProxyService
	botCtx       context.Context
	botCtxCancel context.CancelFunc
	restartChan  chan struct{}
	onRestart    func(context.Context)
	stopOnce     sync.Once
	stopMu       sync.Mutex
	stopped      bool
	activity     atomic.Int64
}

func (s *BotService) Log() *zerolog.Logger {
	return new(log.Logger.With().Str("bot", s.username).Logger())
}

func (s *BotService) Username() string { return s.username }

// Touch records recent bot activity (an incoming update).
func (s *BotService) Touch() {
	s.activity.Store(time.Now().UnixNano())
}

// stalled reports whether no activity was seen for longer than proxyStallTimeout.
func (s *BotService) stalled(now time.Time) bool {
	return now.Sub(time.Unix(0, s.activity.Load())) > proxyStallTimeout
}

func (s *BotService) HealthCheck() error {

	s.stopMu.Lock()
	bot := s.Bot
	botCtx := s.botCtx
	s.stopMu.Unlock()

	if bot == nil {
		return nil
	}
	if _, err := bot.GetMe(botCtx); err != nil {
		return err
	}
	return nil
}

func (s *BotService) Start(ctx context.Context) {

	s.superCtx = ctx

	s.stopMu.Lock()
	s.botCtx, s.botCtxCancel = context.WithCancel(ctx)
	botCtx := s.botCtx
	s.stopMu.Unlock()

	go s.startBot(ctx, botCtx)

	for {
		select {
		case <-ctx.Done():
			s.Log().Debug().Msg("Context cancelled, stopping bot")
			return
		case _, ok := <-s.restartChan:
			if !ok {
				s.Log().Error().Msg("Restart channel closed, stopping bot...")
				return
			}
			s.Log().Trace().Msg("Restarting bot...")
			s.stopMu.Lock()
			cancel := s.botCtxCancel
			s.botCtx, s.botCtxCancel = context.WithCancel(ctx)
			botCtx = s.botCtx
			s.Bot = nil
			s.stopMu.Unlock()

			cancel()
			s.startBot(ctx, botCtx)
		}
	}
}

var restartSF = singleflight.Group{}

func (s *BotService) Restart() {
	restartSF.Do("restart", func() (any, error) {
		s.Log().Trace().Msg("Sending restart signal...")
		s.stopMu.Lock()
		if s.stopped {
			s.stopMu.Unlock()
			s.Log().Debug().Msg("Bot is stopped, skipping restart")
			return nil, nil
		}
		select {
		case s.restartChan <- struct{}{}:
			s.stopMu.Unlock()
			if s.onRestart != nil {
				s.onRestart(s.superCtx)
			}
		default:
			s.stopMu.Unlock()
			s.Log().Debug().Msg("Bot is not ready for restart, skipping")
		}
		return nil, nil
	})
}

func (s *BotService) Stop() {

	s.stopOnce.Do(func() {
		s.Log().Trace().Msg("Stopping bot...")
		s.stopMu.Lock()
		cancel := s.botCtxCancel
		s.stopped = true
		close(s.restartChan)
		s.stopMu.Unlock()

		if cancel != nil {
			cancel()
		}
	})
}

func (s *BotService) OnRestart(f func(context.Context)) { s.onRestart = f }

func (s *BotService) startBot(ctx context.Context, botCtx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		proxy, err := s.proxy.FirstAvailable(ctx)
		if err != nil {
			s.Log().Error().Err(err).Msg("No available proxy, retrying in 10s...")
			time.Sleep(10 * time.Second)

			select {
			case <-ctx.Done():
				s.Log().Error().Msg("Context cancelled, stopping bot...")
				return
			default:
			}
			continue
		}
		s.Log().Debug().Str("proxy", proxy)

		s.Log().Trace().Msg("Building bot...")
		b, err := s.builder(proxy, s.Touch)
		if err != nil {
			s.Log().Error().Err(err).Msg("Failed to build bot, retrying in 5 second...")
			time.Sleep(5 * time.Second)
			continue
		}

		s.Log().Trace().Msg("Bot built, getting username...")
		if me, err := b.GetMe(ctx); err == nil {
			s.username = me.Username
			s.Log().Info().Msg("Bot started")
		} else {
			s.Log().Error().Err(err).Msg("Failed to get bot username, retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		bot.WithErrorsHandler(func(err error) { go s.handleAPIError(err) })(b)
		s.Log().Trace().Msg("Bot initialized")

		s.Touch()
		s.stopMu.Lock()
		s.Bot = b
		bot := s.Bot
		s.stopMu.Unlock()

		go s.runWatchdog(botCtx, bot)

		bot.Start(botCtx)

		return
	}
}

// runWatchdog periodically probes Telegram and restarts the bot with a new
// proxy if the current one stopped delivering updates or became unreachable.
func (s *BotService) runWatchdog(botCtx context.Context, b *bot.Bot) {
	ticker := time.NewTicker(proxyWatchdogPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-botCtx.Done():
			return
		case <-ticker.C:
		}

		ctx, cancel := context.WithTimeout(botCtx, proxyFinderTimeout)
		_, err := b.GetMe(ctx)
		cancel()
		if err != nil {
			s.Log().Error().Err(err).Msg("Watchdog: GetMe failed, restarting with a new proxy...")
			s.Restart()
			return
		}

		if s.stalled(time.Now()) {
			s.Log().Error().Msg("Watchdog: no updates for a long time, restarting with a new proxy...")
			s.Restart()
			return
		}
	}
}

func (s *BotService) handleAPIError(err error) {
	if isTelegramError(err) {
		s.Log().Warn().Err(err).Msg("Telegram API error, no restart needed")
		return
	}
	s.Log().Error().Err(err).Msg("Network error, restarting with a new proxy...")
	s.Restart()
}

func isTelegramError(err error) bool {
	return errors.Is(err, bot.ErrorNotFound) ||
		errors.Is(err, bot.ErrorConflict) ||
		errors.Is(err, bot.ErrorForbidden) ||
		errors.Is(err, bot.ErrorBadRequest) ||
		errors.Is(err, bot.ErrorUnauthorized) ||
		bot.IsTooManyRequestsError(err) ||
		bot.IsMigrateError(err)
}
