package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/robfig/cron"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func NewBroadcastService(bot *BotService, services *Services, reporter reporter.Reporter) *BroadcastService {
	return &BroadcastService{Bot: bot, Services: services, Reporter: reporter, cron: cron.New()}
}

type BroadcastService struct {
	Bot *BotService
	*Services
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

func (s *BroadcastService) Run(ctx context.Context, config BroadcastConfig) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	if config.Daily {
		if err := s.scheduleDaily(s.ctx); err != nil {
			log.Error().Err(err).Msg("Failed to schedule daily broadcast")
			return err
		}
		log.Info().Msg("Daily broadcast scheduled")
	} else {
		log.Info().Msg("Daily broadcast disabled")
	}
	if config.PairNotification {
		if err := s.schedulePairNotification(s.ctx); err != nil {
			log.Error().Err(err).Msg("Failed to schedule pair notification")
			return err
		}
		log.Info().Msg("Pair notification scheduled")
	} else {
		log.Info().Msg("Pair notification disabled")
	}
	if config.ChangeAlert {
		go s.runChangeNotifier(s.ctx)
		log.Info().Msg("Change alert scheduled")
	} else {
		log.Info().Msg("Change alert disabled")
	}

	s.cron.Start()

	return nil
}

// Every minute except Sunday
func (s *BroadcastService) scheduleDaily(ctx context.Context) error {
	return s.cron.AddFunc("0 * * * * 1-6", func() { go s.handleDailyBroadcast(ctx, time.Now()) })
}
func (s *BroadcastService) handleDailyBroadcast(ctx context.Context, t time.Time) {
	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	timeStr := t.Format("15:04")

	start := time.Now()

	chatCount, groupedChats, _, confs, shouldReturn := s.prepareBroadcast(ctx, timeStr, t)
	if shouldReturn {
		return
	}

	// Get schedules
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		if len(schedules) == 0 {
			s.Report().Err(err).Msg("Failed to get schedules for daily broadcast")
			return
		}
		s.Report().Err(err).Debug("success", len(schedules)).Msg("Failed to get some schedules for daily broadcast")
	}

	// Send schedules
	successCount, err := s.sendDaily(ctx, schedules, groupedChats)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	// Log
	if err := s.Log.LogBroadcast(ctx, model.BroadcastLog{
		Kind:    model.BLogDaily,
		Chats:   chatCount,
		Groups:  len(groupedChats),
		Elapsed: int(time.Since(start).Milliseconds()),
		Fails:   chatCount - successCount,
		Errors:  errStr,
	}); err != nil {
		s.Report().Err(err).Msg("Failed to log daily broadcast")
	}
}
func (s *BroadcastService) sendDaily(
	ctx context.Context,
	schedules []*model.RawSchedule,
	groupedChats map[model.GroupName][]*model.Chat,
) (successCount int, err error) {
	var errs []error
	successCount = 0
	for _, schedule := range schedules {
		var err error

		type msgConf struct {
			schedule      model.RawSchedule
			imageFileName string
			imageData     []byte
		}
		confLight := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(false))})
		confLight.imageFileName, confLight.imageData, err = s.Schedule.PrepareScheduleImage(ctx, schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare light schedule image")
			errs = append(errs, err)
			continue
		}

		scheduleDark := schedule.WithConfig(schedule.Config.WithDarkMode(true))
		confDark := new(msgConf{schedule: scheduleDark})
		confDark.imageFileName, confDark.imageData, err = s.Schedule.PrepareScheduleImage(ctx, &scheduleDark)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare dark schedule image")
			errs = append(errs, err)
			continue
		}

		for _, chat := range groupedChats[schedule.Config.Group.GroupName] {
			c := confLight
			if chat.DarkMode {
				c = confDark
			}
			if err := botutil.SendWeekScheduleMessages(
				ctx,
				s.Bot.Bot,
				0,
				chat,
				c.schedule.Config,
				c.imageFileName,
				c.imageData,
			); err != nil {
				log.Error().Err(err).Msg("Failed to send week schedule message")
				errs = append(errs, err)
			} else {
				successCount += 1
			}
		}
	}

	return successCount, errors.Join(errs...)
}

func (s *BroadcastService) schedulePairNotification(ctx context.Context) error {
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
		if err := s.cron.AddFunc(fmt.Sprintf("%d %d * * *", t[1], t[0]), func() {
			go s.handlePairNotification(ctx, time.Now())
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *BroadcastService) handlePairNotification(ctx context.Context, t time.Time) {
	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	timeStart := t.Add(15 * time.Minute)
	timeStartStr := timeStart.Format("15:04")

	start := time.Now()

	chatCount, groupedChats, groupNames, confs, shouldReturn := s.prepareBroadcast(ctx, timeStartStr, t)
	if shouldReturn {
		return
	}

	// Get schedules
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		if len(schedules) == 0 {
			s.Report().Err(err).Msg("Failed to get schedules for daily broadcast")
			return
		}
		s.Report().Err(err).Debug("success", len(schedules)).Msg("Failed to get some schedules for daily broadcast")
	}

	// Send notifications
	successCount, err := s.sendPairNotificatins(ctx, schedules, groupedChats, groupNames, timeStart)

	// Log
	if err := s.Log.LogBroadcast(ctx, model.BroadcastLog{
		Kind:    model.BLogPair,
		Chats:   chatCount,
		Groups:  len(groupedChats),
		Elapsed: int(time.Since(start).Milliseconds()),
		Fails:   chatCount - successCount,
		Errors:  err.Error(),
	}); err != nil {
		s.Report().Err(err).Msg("Failed to log pair notification broadcast")
	}
}
func (s *BroadcastService) sendPairNotificatins(
	ctx context.Context,
	schedules []*model.RawSchedule,
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	t time.Time,
) (successCount int, err error) {
	var errs []error
	successCount = 0
	messagesToDelete := make([]*models.Message, 0)
	for i, schedule := range schedules {
		var err error

		pair, err := schedule.Transform().Days[0].CurrentPair(t)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get current pair: %w", err))
			continue
		}

		switch pair.Kind {
		case model.PairKindEmpty, model.PairKindEvent, model.PairKindIGA, model.PairKindVacation, model.PairKindPractice:
			log.Trace().Str("kind", string(pair.Kind)).Msg("Pair is empty")
			continue
		}
		text := fmt.Sprintf("Следующая пара в кабинете %s:\n\t<b>%s</b>\n\t%s",
			pair.Classroom, pair.Discipline, refutil.DerefOrTypeDefault(pair.Teacher))

		for _, chat := range groupedChats[groupNames[i]] {
			if msg, err := s.Bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				Text:            text,
				ParseMode:       models.ParseModeHTML,
			}); err != nil {
				errs = append(errs, fmt.Errorf("failed to send pair notification: %w", err))
			} else {
				messagesToDelete = append(messagesToDelete, msg)
			}
		}
	}

	go func(msgs []*models.Message) {
		time.Sleep(viper.GetDuration(config.KeyPairNotificationTTL))
		for _, m := range msgs {
			if _, err := s.Bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: m.Chat.ID, MessageID: m.ID}); err != nil {
				log.Error().Err(err).Any("message", m).Msg("Failed to delete pair notification message")
			}
		}
	}(messagesToDelete)

	return successCount, errors.Join(errs...)
}

func (s *BroadcastService) runChangeNotifier(ctx context.Context) {
	halfInterval := viper.GetDuration(config.KeyUpdateMonitorInterval) / 2
	log.Info().Dur("halfInterval", halfInterval).Msg("Change alert notifier will start in half of the interval")
	time.Sleep(halfInterval)

	log.Info().Msg("Change alert notifier started")
	for {
		if s.Bot == nil {
			log.Warn().Msg("Bot is not initialized")
			time.Sleep(5 * time.Second)
			continue
		}

		select {
		case <-ctx.Done():
			log.Info().Msg("Change alert notifier stopped")
			return
		default:
			s.handleChangeAlert(ctx)
			time.Sleep(viper.GetDuration(config.KeyUpdateMonitorInterval))
		}
	}
}
func (s *BroadcastService) handleChangeAlert(ctx context.Context) {
	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	start := time.Now()

	chatCount, groupedChats, groupNames, confs, shouldReturn := s.prepareBroadcast(ctx, start.Format("15:04"), start)
	if shouldReturn {
		return
	}

	// Get changes
	changes, err := s.Schedule.GetChanges(ctx, groupNames)
	if err != nil {
		if len(changes) == 0 {
			s.Report().Err(err).Msg("Failed to get schedule changes")
			return
		}
		s.Report().Err(err).Debug("success", len(changes)).Msg("Failed to get schdule changes for some groups")
	}

	// Get schedules
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		if len(schedules) == 0 {
			s.Report().Err(err).Msg("Failed to get schedules for change alert")
			return
		}
		s.Report().Err(err).Debug("success", len(schedules)).Msg("Failed to get some schedules for change alert")
	}

	// Send reports
	successCount, err := s.sendChangeReports(ctx, schedules, groupedChats, changes)

	// Log
	if err := s.Log.LogBroadcast(ctx, model.BroadcastLog{
		Kind:    model.BLogChange,
		Chats:   chatCount,
		Groups:  len(groupedChats),
		Elapsed: int(time.Since(start).Milliseconds()),
		Fails:   chatCount - successCount,
		Errors:  err.Error(),
	}); err != nil {
		s.Report().Err(err).Msg("Failed to log change alert broadcast")
	}
}
func (s *BroadcastService) sendChangeReports(
	ctx context.Context,
	schedules []*model.RawSchedule,
	groupedChats map[model.GroupName][]*model.Chat,
	changes map[model.GroupName]*model.ScheduleChange,
) (successCount int, err error) {
	var errs []error
	successCount = 0
	for _, schedule := range schedules {
		type msgConf struct {
			schedule      model.RawSchedule
			imageFileName string
			imageData     []byte
		}
		light := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(false))})
		light.imageFileName, light.imageData, err = s.Schedule.PrepareScheduleImage(ctx, schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare light schedule image")
			errs = append(errs, err)
			continue
		}

		dark := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(true))})
		dark.imageFileName, dark.imageData, err = s.Schedule.PrepareScheduleImage(ctx, schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare dark schedule image")
			errs = append(errs, err)
			continue
		}

		text := changes[schedule.Config.Group.GroupName].HTML()

		for _, chat := range groupedChats[schedule.Config.Group.GroupName] {
			c := light
			if chat.DarkMode {
				c = dark
			}

			if err := botutil.SendWeekScheduleMessages(
				ctx,
				s.Bot.Bot,
				0,
				chat,
				c.schedule.Config,
				c.imageFileName,
				c.imageData,
			); err != nil {
				log.Error().Err(err).Msg("Failed to send week schedule messages")
				errs = append(errs,
					fmt.Errorf("failed to send week schedule messages for chat %d: %w", chat.TgChatID, err))
			}

			if _, err := s.Bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				ParseMode:       models.ParseModeHTML,
				Text:            text,
			}); err != nil {
				log.Error().Err(err).Msg("Failed to send schedule change alert message")
				errs = append(errs, err)
				continue
			}

			successCount++
		}
	}

	return successCount, errors.Join(errs...)
}

func (s *BroadcastService) prepareBroadcast(ctx context.Context, timeStr string, t time.Time) (
	chatCount int,
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	confs []model.ScheduleConfig,
	shouldReturn bool,
) {
	chats, err := s.Chat.GetChatsByDailyTime(ctx, timeStr)
	if err != nil {
		s.Report().Err(err).Debug("time", timeStr).Msg("Failed to get chats for broadcast")
		return 0, nil, nil, nil, true
	}

	chatCount = len(chats)
	if chatCount == 0 {
		log.Debug().Time("time", t).Msg("No chat for broadcast")
		return 0, nil, nil, nil, true
	}

	groupedChats = groupChats(chats)
	groupCount := len(groupedChats)
	log.Info().Time("time", t).Int("chats", chatCount).Int("groups", groupCount).Msg("Processing broadcast...")

	// Prepare schedule configs
	groupNames = make([]model.GroupName, 0, len(groupedChats))
	confs = make([]model.ScheduleConfig, 0, len(groupedChats))
	for gn := range groupedChats {
		group, err := s.Schedule.GetGroupByName(ctx, gn)
		if err != nil {
			log.Error().Err(err).Str("group", string(gn)).Msg("Failed to get group by name")
			continue
		}
		groupNames = append(groupNames, gn)
		confs = append(confs, model.ScheduleConfig{Group: group})
	}
	if len(confs) == 0 {
		log.Warn().Msg("No valid groups for daily broadcast")
		return 0, nil, nil, nil, true
	}
	return chatCount, groupedChats, groupNames, confs, shouldReturn
}

func groupChats(chats []*model.Chat) map[model.GroupName][]*model.Chat {
	grouped := make(map[model.GroupName][]*model.Chat)
	for _, chat := range chats {
		if chat.GroupName == nil {
			log.Warn().Msg("Chat has no group name")
			continue
		}
		grouped[*chat.GroupName] = append(grouped[*chat.GroupName], chat)
	}
	return grouped
}

func (s *BroadcastService) Stop() {
	s.cancel()
	s.cron.Stop()
}
