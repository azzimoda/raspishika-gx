package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func NewBroadcastService(bot *BotService, services *Services, reporter reporter.Reporter) *BroadcastService {
	return &BroadcastService{Bot: bot, Services: services, Reporter: reporter, cron: cron.New(cron.WithSeconds())}
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
	_, err := s.cron.AddFunc("0 * * * 1-6,9-12 *", func() { go s.handleDailyBroadcast(ctx, time.Now()) })
	return err
}
func (s *BroadcastService) handleDailyBroadcast(ctx context.Context, t time.Time) {
	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	timeStr := t.Format("15:04")
	start := time.Now()
	if start.Month() == time.July || start.Month() == time.August {
		log.Debug().Msg("Skipping daily broadcast during summer")
		return
	}

	groupedChats, _, confs, shouldReturn := s.prepareBroadcast(ctx, timeStr, t)
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

	// Create task log
	taskLog := model.BroadcastTaskLog{Kind: model.BDaily}
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
		return
	}

	// Send schedules
	err = s.sendDaily(ctx, taskLog.ID, schedules, groupedChats)
	if err != nil {
		log.Error().Err(err).Msg("Errors while sending daily broadcast")
	}

	// Update task log
	taskLog.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.UpdateBroadcastTaskLog(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to update broadcast task log")
	}
}
func (s *BroadcastService) sendDaily(
	ctx context.Context,
	taskID int64,
	schedules []*model.RawSchedule,
	groupedChats map[model.GroupName][]*model.Chat,
) (err error) {
	var errs []error
	successCount := 0
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
			var err error
			if err = botutil.SendWeekScheduleMessages(
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
				successCount++
			}

			s.logBroadcast(ctx, taskID, chat, err)
		}
	}

	log.Debug().Int("successCount", successCount).Msg("Daily sending finished")

	return errors.Join(errs...)
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
		if _, err := s.cron.AddFunc(fmt.Sprintf("0 %d %d * 1-6,9-12 1-6", t[1], t[0]), func() {
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

	groupedChats, groupNames, confs, shouldReturn := s.prepareBroadcast(ctx, timeStartStr, t)
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

	// Create task log
	taskLog := model.BroadcastTaskLog{Kind: model.BPair}
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
		return
	}

	// Send notifications
	err = s.sendPairNotificatins(ctx, taskLog.ID, schedules, groupedChats, groupNames, timeStart)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send pair notifications")
	}

	// Log
	taskLog.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
	}
}
func (s *BroadcastService) sendPairNotificatins(
	ctx context.Context,
	taskID int64,
	schedules []*model.RawSchedule,
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	t time.Time,
) (err error) {
	var errs []error
	successCount := 0
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
			msg, err := s.Bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				Text:            text,
				ParseMode:       models.ParseModeHTML,
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to send pair notification: %w", err))
			} else {
				messagesToDelete = append(messagesToDelete, msg)
			}

			s.logBroadcast(ctx, taskID, chat, err)
		}
	}

	log.Info().Int("successCount", successCount).Msg("Pair notifications sent")

	go func(msgs []*models.Message) {
		time.Sleep(viper.GetDuration(config.KeyPairNotificationTTL))
		for _, m := range msgs {
			if _, err := s.Bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: m.Chat.ID, MessageID: m.ID}); err != nil {
				log.Error().Err(err).Any("message", m).Msg("Failed to delete pair notification message")
			}
		}
	}(messagesToDelete)

	return errors.Join(errs...)
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

		if t := time.Now(); t.Month() == time.July || t.Month() == time.August {
			log.Debug().Msg("Change alert notifier is disabled during summer")
			time.Sleep(viper.GetDuration(config.KeyUpdateMonitorInterval))
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

	groupedChats, groupNames, confs, shouldReturn := s.prepareBroadcast(ctx, start.Format("15:04"), start)
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

	// Create task log
	taskLog := model.BroadcastTaskLog{Kind: model.BChange}
	err = s.Stats.LogBroadcastTask(ctx, &taskLog)
	if err != nil {
		s.Report().Err(err).Msg("Failed to start broadcast task")
		return
	}

	// Send reports
	_, err = s.sendChangeReports(ctx, taskLog.ID, schedules, groupedChats, changes)
	if err != nil {
		log.Error().Err(err).Msg("Errors while sending change reports")
	}

	// Log
	taskLog.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
	}
}
func (s *BroadcastService) sendChangeReports(
	ctx context.Context,
	taskID int64,
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

			var err error = nil

			if errSchedule := botutil.SendWeekScheduleMessages(
				ctx,
				s.Bot.Bot,
				0,
				chat,
				c.schedule.Config,
				c.imageFileName,
				c.imageData,
			); errSchedule != nil {
				err = errSchedule
				log.Error().Err(err).Msg("Failed to send week schedule messages")
			}

			if _, errReport := s.Bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				ParseMode:       models.ParseModeHTML,
				Text:            text,
			}); errReport != nil {
				err = errors.Join(err, errReport)
				log.Error().Err(errReport).Msg("Failed to send schedule change alert message")
			} else {
				successCount++
			}

			s.logBroadcast(ctx, taskID, chat, err)
		}
	}

	return successCount, errors.Join(errs...)
}

func (s *BroadcastService) logBroadcast(ctx context.Context, taskID int64, chat *model.Chat, err error) {
	var errVal *string = nil
	if err != nil {
		errVal = new(err.Error())
	}
	if err := s.Stats.LogBroadcast(ctx, model.BroadcastLog{
		TaskID: taskID,
		ChatID: int64(chat.TgChatID),
		Error:  errVal,
	}); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast")
	}
}

func (s *BroadcastService) prepareBroadcast(ctx context.Context, timeStr string, t time.Time) (
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	confs []model.ScheduleConfig,
	shouldReturn bool,
) {
	chats, err := s.Chat.GetChatsByDailyTime(ctx, timeStr)
	if err != nil {
		s.Report().Err(err).Debug("time", timeStr).Msg("Failed to get chats for broadcast")
		return nil, nil, nil, true
	}

	chatCount := len(chats)
	if chatCount == 0 {
		log.Debug().Time("time", t).Msg("No chat for broadcast")
		return nil, nil, nil, true
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
		return nil, nil, nil, true
	}
	return groupedChats, groupNames, confs, shouldReturn
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
