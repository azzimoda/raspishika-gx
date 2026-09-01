package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/azzimoda/go-tg-proxy/botservice"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func NewBroadcastService(bot *botservice.BotService, services *Services, reporter reporter.Reporter) *BroadcastService {
	return &BroadcastService{Bot: bot, Services: services, Reporter: reporter, cron: cron.New(cron.WithSeconds())}
}

type BroadcastService struct {
	Bot *botservice.BotService
	*Services
	reporter.Reporter
	cron   *cron.Cron
	ctx    context.Context
	cancel context.CancelFunc
	jobs   jobGroup
}

// jobGroup tracks in-flight goroutines so shutdown can wait for them
// without the WaitGroup Add/Wait reuse restrictions.
type jobGroup struct {
	mu     sync.Mutex
	n      int
	waitCh chan struct{}
}

func (g *jobGroup) add() {

	g.mu.Lock()
	g.n++
	g.mu.Unlock()
}

func (g *jobGroup) done() {

	g.mu.Lock()
	g.n--
	if g.n == 0 && g.waitCh != nil {
		close(g.waitCh)
		g.waitCh = nil
	}
	g.mu.Unlock()
}

// wait blocks until all jobs started so far finish, or until ctx is done.
func (g *jobGroup) wait(ctx context.Context) {

	g.mu.Lock()
	if g.n == 0 {
		g.mu.Unlock()
		return
	}
	ch := make(chan struct{})
	g.waitCh = ch
	g.mu.Unlock()

	select {
	case <-ch:
	case <-ctx.Done():
		log.Warn().Err(ctx.Err()).Msg("Timed out waiting for broadcast jobs")
	}
}

// runJob spawns f as a tracked goroutine.
func (s *BroadcastService) runJob(f func()) {

	s.jobs.add()
	go func() {
		defer s.jobs.done()
		f()
	}()
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
		s.runJob(func() { s.runChangeNotifier(s.ctx) })
		log.Info().Msg("Change alert scheduled")
	} else {
		log.Info().Msg("Change alert disabled")
	}

	s.cron.Start()

	return nil
}

// Every minute
func (s *BroadcastService) scheduleDaily(ctx context.Context) error {

	_, err := s.cron.AddFunc("0 * * * * *", func() {
		s.runJob(func() { s.handleDailyBroadcast(ctx, time.Now()) })
	})
	return err
}
func (s *BroadcastService) handleDailyBroadcast(ctx context.Context, t time.Time) {

	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	if status, err := s.Schedule.IsVacation(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to check vacation status; try to process anyway...")
	} else if status {
		log.Debug().Msg("Skipped daily broadcast because of vacation")
		return
	}

	timeStr := t.Format("15:04")
	start := time.Now()

	chats, err := s.Chat.GetChatsByDailyTime(ctx, timeStr)
	if err != nil {
		log.Error().Err(err).Str("time", timeStr).Msg("Failed to get chats for daily broadcast")
		return
	}

	groupedChats, groups, confs, shouldReturn := s.prepareBroadcast(ctx, chats)
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
	taskLog := model.BroadcastTaskLog{Kind: model.BDaily, Groups: len(groups)}
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
	schedules []*model.ScheduleData,
	groupedChats map[model.GroupName][]*model.Chat,
) (err error) {
	var errs []error
	for _, schedule := range schedules {
		if schedule == nil {
			log.Warn().Msg("Nil schedule in daily broadcast")
			continue
		}

		var err error

		type msgConf struct {
			schedule      model.ScheduleData
			imageFileName string
			imageData     []byte
		}
		light := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(false))})
		light.imageFileName, light.imageData, err = s.Schedule.PrepareScheduleImage(ctx, &light.schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare light schedule image")
			errs = append(errs, err)
			continue
		}

		dark := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(true))})
		dark.imageFileName, dark.imageData, err = s.Schedule.PrepareScheduleImage(ctx, &dark.schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare dark schedule image")
			errs = append(errs, err)
			continue
		}

		for _, chat := range groupedChats[schedule.Config.Group.GroupName] {
			c := light
			if chat.DarkMode {
				c = dark
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
				botutil.SchedulePageURL(c.schedule.Config, nil),
				c.schedule.IsOld,
			); err != nil {
				errs = append(errs, err)
				if errors.Is(err, bot.ErrorForbidden) {
					s.handleForbidden(ctx, err, chat)
					continue
				}
				log.Error().Err(err).Msg("Failed to send week schedule message")
			}

			s.logBroadcast(ctx, taskID, chat, err)
		}
	}

	log.Debug().Msg("Daily sending finished")

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
		if _, err := s.cron.AddFunc(fmt.Sprintf("0 %d %d * * 1-6", t[1], t[0]), func() {
			s.runJob(func() { s.handlePairNotification(ctx, time.Now()) })
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

	if status, err := s.Schedule.IsVacation(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to check vacation status; try to process anyway...")
	} else if status {
		log.Debug().Msg("Skipped pair notification because of vacation")
		return
	}

	start := time.Now()

	pairTimeStart := t.Add(15 * time.Minute)
	pairTimeStartStr := pairTimeStart.Format("15:04")

	chats, err := s.Chat.GetChatsWithPairNotification(ctx)
	if err != nil {
		log.Error().Err(err).Str("time", pairTimeStartStr).Msg("Failed to get chats for pair notificatoin")
		return
	}

	groupedChats, groups, confs, shouldReturn := s.prepareBroadcast(ctx, chats)
	if shouldReturn {
		return
	}

	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		s.Report().Err(err).Debug("success", len(schedules)).Msg("Failed to get some schedules for daily broadcast")
	}

	taskLog := model.BroadcastTaskLog{Kind: model.BPair, Groups: len(groups)}
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
		return
	}

	err = s.sendPairNotificatins(ctx, taskLog.ID, schedules, groupedChats, groups, pairTimeStart)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send pair notifications")
	}

	taskLog.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.UpdateBroadcastTaskLog(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
	}
}
func (s *BroadcastService) sendPairNotificatins(
	ctx context.Context,
	taskID int64,
	schedules []*model.ScheduleData,
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	t time.Time,
) (err error) {
	var errs []error
	successCount := 0
	messagesToDelete := make([]*models.Message, 0)
	for i, schedule := range schedules {
		if schedule == nil {
			log.Warn().Msg("Nil schedule in pair notificatoin")
			continue
		}

		var err error

		pair, err := schedule.Days[0].CurrentPair(t)
		if err != nil {
			if errors.Is(err, model.ErrAllPairsPassed) {
				continue
			}

			errs = append(errs, fmt.Errorf("failed to get current pair: %w", err))
			continue
		}

		switch pair.Kind {
		case model.PairKindEmpty, model.PairKindEvent, model.PairKindIGA, model.PairKindVacation, model.PairKindPractice:
			log.Trace().Str("kind", string(pair.Kind)).Msg("Pair is empty")
			continue
		}
		text := fmt.Sprintf("Следующая пара в кабинете %s:\n\t<b>%s</b>\n\t%s",
			pair.Classroom, pair.Discipline, pair.Teacher)

		for _, chat := range groupedChats[groupNames[i]] {
			msg, err := s.Bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				Text:            text,
				ParseMode:       models.ParseModeHTML,
			})
			if err != nil {
				errs = append(errs, err)
				if errors.Is(err, bot.ErrorForbidden) {
					s.handleForbidden(ctx, err, chat)
					continue
				}
			} else {
				messagesToDelete = append(messagesToDelete, msg)
			}

			s.logBroadcast(ctx, taskID, chat, err)
		}
	}

	log.Info().Int("successCount", successCount).Msg("Pair notifications sent")

	s.runJob(func() {
		select {
		case <-time.After(viper.GetDuration(config.KeyPairNotificationTTL)):
		case <-ctx.Done():
			return
		}
		for _, m := range messagesToDelete {
			if _, err := s.Bot.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: m.Chat.ID, MessageID: m.ID}); err != nil {
				log.Error().Err(err).Any("message", m).Msg("Failed to delete pair notification message")
			}
		}
	})

	return errors.Join(errs...)
}

func (s *BroadcastService) runChangeNotifier(ctx context.Context) {

	halfInterval := viper.GetDuration(config.KeyUpdateMonitorInterval) / 2
	log.Info().Dur("halfInterval", halfInterval).Msg("Change alert notifier will start in half of the interval")
	if !sleepContext(ctx, halfInterval) {
		log.Info().Msg("Change alert notifier stopped")
		return
	}

	log.Info().Msg("Change alert notifier started")
	for {
		if s.Bot == nil {
			log.Warn().Msg("Bot is not initialized")
			if !sleepContext(ctx, 5*time.Second) {
				break
			}
			continue
		}

		if status, err := s.Schedule.IsVacation(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to check vacation status; try to process anyway...")
		} else if status {
			log.Debug().Msg("Change alert is skipped because of vacation")
			if !sleepContext(ctx, viper.GetDuration(config.KeyUpdateMonitorInterval)) {
				break
			}
			continue
		}

		s.handleChangeAlert(ctx)
		if !sleepContext(ctx, viper.GetDuration(config.KeyUpdateMonitorInterval)) {
			break
		}
	}
	log.Info().Msg("Change alert notifier stopped")
}

// sleepContext sleeps for d unless ctx is cancelled, in which case it returns false.
func sleepContext(ctx context.Context, d time.Duration) bool {

	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
func (s *BroadcastService) handleChangeAlert(ctx context.Context) {

	if s.Bot == nil {
		log.Warn().Msg("Bot is not initialized")
		return
	}

	start := time.Now()

	chats, err := s.Chat.GetChatsWithChangeAlert(ctx)
	if err != nil {
		s.Report().Err(err).Msg("Failed to get chats for change alert")
		return
	}

	groupedChats, groups, confs, shouldReturn := s.prepareBroadcast(ctx, chats)
	if shouldReturn {
		return
	}

	changes, err := s.Schedule.GetChanges(ctx, groups)
	if err != nil {
		if len(changes) == 0 {
			s.Report().Err(err).Msg("Failed to get schedule changes")
			return
		}
		s.Report().Err(err).Debug("success", len(changes)).Msg("Failed to get schdule changes for some groups")
	}

	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		if len(schedules) == 0 {
			s.Report().Err(err).Msg("Failed to get schedules for change alert")
			return
		}
		s.Report().Err(err).Debug("success", len(schedules)).Msg("Failed to get some schedules for change alert")
	}

	taskLog := model.BroadcastTaskLog{Kind: model.BChange, Groups: len(groups)}
	err = s.Stats.LogBroadcastTask(ctx, &taskLog)
	if err != nil {
		s.Report().Err(err).Msg("Failed to start broadcast task")
		return
	}

	_, err = s.sendChangeReports(ctx, taskLog.ID, schedules, groupedChats, changes)
	if err != nil {
		log.Error().Err(err).Msg("Errors while sending change reports")
	}

	taskLog.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.UpdateBroadcastTaskLog(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
	}
}
func (s *BroadcastService) sendChangeReports(
	ctx context.Context,
	taskID int64,
	schedules []*model.ScheduleData,
	groupedChats map[model.GroupName][]*model.Chat,
	changes map[model.GroupName]*model.ScheduleChange,
) (successCount int, err error) {
	var errs []error
	successCount = 0
	for _, schedule := range schedules {
		if schedule == nil {
			log.Warn().Msg("Nil schedule in change alert")
			continue
		}

		sch, ok := changes[schedule.Config.Group.GroupName]
		if !ok {
			log.Error().Str("group", string(schedule.Config.Group.GroupName)).
				Msg("Group schedule changes not found; supposed to be unreachable")
			continue
		}
		if sch == nil {
			log.Warn().Str("group", string(schedule.Config.Group.GroupName)).
				Msg("Group scheduel changes are nil; supposed to be unreachable")
			continue
		}

		type msgConf struct {
			schedule      model.ScheduleData
			imageFileName string
			imageData     []byte
		}
		light := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(false))})
		light.imageFileName, light.imageData, err = s.Schedule.PrepareScheduleImage(ctx, &light.schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare light schedule image")
			errs = append(errs, err)
			continue
		}

		dark := new(msgConf{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(true))})
		dark.imageFileName, dark.imageData, err = s.Schedule.PrepareScheduleImage(ctx, &dark.schedule)
		if err != nil {
			log.Error().Err(err).Msg("Failed to prepare dark schedule image")
			errs = append(errs, err)
			continue
		}
		text := sch.HTML()

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
				botutil.SchedulePageURL(c.schedule.Config, nil),
				c.schedule.IsOld,
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
				if errors.Is(errReport, bot.ErrorForbidden) {
					s.handleForbidden(ctx, errReport, chat)
					continue
				}
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
		Group:  refutil.DerefOrTypeDefault(chat.GroupName),
		Error:  errVal,
	}); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast")
	}
}

// BroadcastText sends the given HTML message to every chat in chats as a mass
// broadcast task, asynchronously and tracked by the service so shutdown waits
// for it to finish. Returns only synchronous setup errors; per-chat send
// outcomes are reported via the service reporter.
func (s *BroadcastService) BroadcastText(ctx context.Context, chats []*model.Chat, htmlText string) error {
	if s.Bot == nil {
		return fmt.Errorf("bot is not initialized")
	}
	if len(chats) == 0 {
		log.Debug().Msg("No chats for mass broadcast")
		return nil
	}

	taskLog := model.BroadcastTaskLog{Kind: model.BMass, Groups: 1}
	if err := s.Stats.LogBroadcastTask(ctx, &taskLog); err != nil {
		s.Report().Err(err).Msg("Failed to log broadcast task")
		return err
	}

	// Run the send under the service lifecycle context so shutdown can cancel
	// an in-flight send and Stop() waits for the job to finish.
	taskCtx := ctx
	if taskCtx == nil {
		taskCtx = s.ctx
	}
	start := time.Now()

	s.runJob(func() {
		var successCount int
		var errs []error
		for _, chat := range chats {
			if chat == nil {
				continue
			}
			var sendErr error
			if _, sendErr = s.Bot.SendMessage(taskCtx, &bot.SendMessageParams{
				ChatID:          chat.TgChatID,
				MessageThreadID: 0,
				Text:            htmlText,
				ParseMode:       models.ParseModeHTML,
			}); sendErr != nil {
				if errors.Is(sendErr, bot.ErrorForbidden) {
					s.handleForbidden(taskCtx, sendErr, chat)
				} else {
					errs = append(errs, sendErr)
					log.Error().Err(sendErr).Msg("Failed to send mass broadcast message")
				}
			} else {
				successCount++
			}
			s.logBroadcast(taskCtx, taskLog.ID, chat, sendErr)
		}

		taskLog.Elapsed = time.Since(start).Milliseconds()
		if err := s.Stats.UpdateBroadcastTaskLog(taskCtx, &taskLog); err != nil {
			s.Report().Err(err).Msg("Failed to update broadcast task log")
		}

		if len(errs) > 0 {
			s.Report().Err(errors.Join(errs...)).Debug("success", successCount).Msg("Mass broadcast finished with errors")
		} else {
			s.Report().Debug("success", successCount).Msg("Mass broadcast finished")
		}
	})

	log.Debug().Msg("Mass broadcast started in background")
	return nil
}

func (s *BroadcastService) prepareBroadcast(ctx context.Context, chats []*model.Chat) (
	groupedChats map[model.GroupName][]*model.Chat,
	groupNames []model.GroupName,
	confs []model.ScheduleConfig,
	shouldReturn bool,
) {
	chatCount := len(chats)
	if chatCount == 0 {
		log.Debug().Msg("No chat for broadcast")
		return nil, nil, nil, true
	}

	groupedChats = groupChats(chats)
	log.Debug().Int("chats", chatCount).Int("groups", len(groupedChats)).Msg("Processing broadcast...")

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

func (s *BroadcastService) Stop(ctx context.Context) {

	if s.cancel != nil {
		s.cancel()
	}
	_ = s.cron.Stop()
	s.jobs.wait(ctx)
}

func (s *BroadcastService) handleForbidden(ctx context.Context, err error, chat *model.Chat) {

	s.Report().Err(err).Debug("chatID", chat.TgChatID).Msg("Bot was kicked from the chat")
	if err := s.Chat.DeleteChat(ctx, chat.ID); err != nil {
		s.Report().Err(err).Debug("chatID", chat.TgChatID).Msg("Failed to delete chat")
	}
}
