package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/robfig/cron"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/reporter"
)

func NewBroadcastService(bot *bot.Bot, services *Services, reporter reporter.Reporter) *BroadcastService {
	return &BroadcastService{Bot: bot, services: services, Reporter: reporter, cron: cron.New()}
}

type BroadcastService struct {
	*bot.Bot
	services *Services
	reporter.Reporter
	cron   *cron.Cron
	ctx    context.Context
	cancel context.CancelFunc
}

type BroadcastConfig struct {
	Daily            bool
	PairNotification bool
	ChangeAlert      bool
}

func (s *BroadcastService) Run(ctx context.Context, config BroadcastConfig) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	if config.Daily {
		s.scheduleDaily(s.ctx)
		log.Info().Msg("Daily broadcast scheduled")
	}
	if config.PairNotification {
		s.SchedulePairNotification(s.ctx)
		log.Info().Msg("Pair notification scheduled")
	}
	if config.ChangeAlert {
		go s.RunChangeAlert(s.ctx)
		log.Info().Msg("Change alert scheduled")
	}

	s.cron.Start()
}

func (s *BroadcastService) scheduleDaily(ctx context.Context) {
	// Every minute
	s.cron.AddFunc("* * * * *", func() { go s.handleDailyBroadcast(ctx, time.Now()) })
}
func (s *BroadcastService) handleDailyBroadcast(ctx context.Context, t time.Time) {
	log.Info().Time("t", t).Msg("Daily broadcast")
	// TODO
}

func (s *BroadcastService) SchedulePairNotification(ctx context.Context) {
	times := [][]int{
		{7, 45},  // 8:00
		{9, 30},  // 9:45
		{11, 15}, // 11:30
		{13, 30}, // 13:45
		{15, 15}, // 15:30
		{17, 0},  // 17:15
		{18, 45}, // 19:00
		// 15 minutes before a pair starts
	}
	for _, t := range times {
		s.cron.AddFunc(fmt.Sprintf("%d %d * * *", t[1], t[0]), func() {
			go s.handlePairNotification(time.Now())
		})
	}
}
func (s *BroadcastService) handlePairNotification(t time.Time) {
	log.Info().Time("t", t).Msg("Pair notification")
	// TODO
}

func (s *BroadcastService) RunChangeAlert(ctx context.Context) {
	// TODO
}

func (s *BroadcastService) Stop() {
	s.cancel()
	s.cron.Stop()
}
