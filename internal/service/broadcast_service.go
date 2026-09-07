package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sync"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/messenger"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// NewBroadcastService returns a messenger-neutral broadcast service. Text
// passed to the messenger is Telegram HTML; the messenger adapter renders it
// for a specific platform.
func NewBroadcastService(messenger messenger.Messenger, services *Services, report reporter.Reporter) *BroadcastService {
	ctx, cancel := context.WithCancel(context.Background())
	return &BroadcastService{
		Messenger: messenger, Services: services, Reporter: report,
		cron: cron.New(cron.WithSeconds()), ctx: ctx, cancel: cancel,
	}
}

type BroadcastService struct {
	Messenger messenger.Messenger
	*Services
	reporter.Reporter
	cron       *cron.Cron
	ctx        context.Context
	cancel     context.CancelFunc
	parentStop func() bool
	mu         sync.Mutex
	running    bool
	stopped    bool
	jobs       jobGroup
}

// jobGroup allows concurrent waiters and rejects new jobs after shutdown begins.
type jobGroup struct {
	mu     sync.Mutex
	n      int
	closed bool
	waitCh chan struct{}
}

func (g *jobGroup) add() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	if g.n == 0 {
		g.waitCh = make(chan struct{})
	}
	g.n++
	return true
}

func (g *jobGroup) done() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n--
	if g.n == 0 {
		close(g.waitCh)
	}
}

func (g *jobGroup) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

func (g *jobGroup) wait(ctx context.Context) {
	g.mu.Lock()
	if g.n == 0 {
		g.mu.Unlock()
		return
	}
	ch := g.waitCh
	g.mu.Unlock()
	select {
	case <-ch:
	case <-ctx.Done():
		log.Warn().Err(ctx.Err()).Msg("Timed out waiting for broadcast jobs")
	}
}

func (s *BroadcastService) runJob(f func()) bool {
	if !s.jobs.add() {
		return false
	}
	go func() { defer s.jobs.done(); f() }()
	return true
}

type BroadcastConfig struct {
	Daily            bool
	PairNotification bool
	ChangeAlert      bool
}

func (s *BroadcastService) Run(ctx context.Context, conf BroadcastConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("broadcast service has stopped")
	}
	if s.running {
		return errors.New("broadcast service is already running")
	}
	if ctx == nil {
		return errors.New("broadcast context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Messenger == nil {
		return errors.New("messenger is not initialized")
	}
	if s.Services == nil || s.Chat == nil || s.Stats == nil || s.Schedule == nil {
		return errors.New("broadcast services are not initialized")
	}
	if conf.Daily {
		if err := s.scheduleDaily(s.ctx); err != nil {
			return err
		}
	}
	if conf.PairNotification {
		if err := s.schedulePairNotification(s.ctx); err != nil {
			return err
		}
	}
	s.parentStop = context.AfterFunc(ctx, s.cancel)
	if conf.ChangeAlert {
		s.runJob(func() { s.runChangeNotifier(s.ctx) })
	}
	s.running = true
	s.cron.Start()
	log.Info().Bool("daily", conf.Daily).Bool("pair", conf.PairNotification).
		Bool("changes", conf.ChangeAlert).Msg("Broadcasts started")
	return nil
}

func (s *BroadcastService) scheduleDaily(ctx context.Context) error {
	_, err := s.cron.AddFunc("0 * * * * *", func() {
		s.runJob(func() { s.handleDailyBroadcast(ctx, time.Now()) })
	})
	return err
}

func (s *BroadcastService) canBroadcast(ctx context.Context) bool {
	if ctx.Err() != nil || s.Messenger == nil {
		return false
	}
	if vacation, err := s.Schedule.IsVacation(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to check vacation status; processing broadcasts")
	} else if vacation {
		log.Debug().Msg("Skipped broadcast because of vacation")
		return false
	}
	return true
}

func (s *BroadcastService) handleDailyBroadcast(ctx context.Context, t time.Time) {
	if !s.canBroadcast(ctx) {
		return
	}
	start := time.Now()
	chats, err := s.Chat.GetChatsByDailyTime(ctx, t.Format("15:04"))
	if err != nil {
		s.reportError(err, "Failed to get chats for daily broadcast")
		return
	}
	grouped, groups, confs, invalid, stop := s.prepareBroadcast(ctx, chats)
	s.notifyAndResetInvalidChats(ctx, invalid)
	if stop {
		return
	}
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		s.reportError(err, "Failed to get some schedules for daily broadcast")
	}
	if len(schedules) == 0 {
		return
	}
	task := model.BroadcastTaskLog{Kind: model.BDaily, Groups: len(groups)}
	if err := s.Stats.LogBroadcastTask(ctx, &task); err != nil {
		s.reportError(err, "Failed to log daily broadcast task")
		return
	}
	if err := s.sendDaily(ctx, task.ID, schedules, grouped); err != nil {
		s.reportError(err, "Errors while sending daily broadcast")
	}
	s.finishTask(ctx, &task, start)
}

type broadcastImage struct {
	schedule model.ScheduleData
	filename string
	data     []byte
}

// Each variant owns its config value; preparing one must not switch another
// recipient's theme or mutate the schedule fetched from the shared cache.
func (s *BroadcastService) prepareImage(ctx context.Context, schedule *model.ScheduleData, dark bool) (*broadcastImage, error) {
	result := &broadcastImage{schedule: schedule.WithConfig(schedule.Config.WithDarkMode(dark))}
	var err error
	result.filename, result.data, err = s.Schedule.PrepareScheduleImage(ctx, &result.schedule)
	return result, err
}

func (s *BroadcastService) sendSchedule(ctx context.Context, chat *model.Chat, img *broadcastImage) error {
	caption := img.schedule.Config.FormatHTML()
	if img.schedule.IsOld {
		caption += "\nНе удалось обновить расписание: информация может быть неактуальной."
	}
	if url := model.ScheduleURL(img.schedule.Config, nil); url != "" {
		caption += fmt.Sprintf("\n<a href=\"%s\">Открыть на сайте</a>", html.EscapeString(url))
	}
	return s.Messenger.SendPhotoPeer(ctx, int64(chat.PeerID), img.filename, img.data, caption)
}

// currentRecipient resolves queued recipients by database identity immediately
// before a send. A queue snapshot must not revive a chat removed by /stop or
// an opt-in which was changed while another recipient or a screenshot was being
// processed.
func (s *BroadcastService) currentRecipient(ctx context.Context, queued *model.Chat, kind model.BroadcastKind) (*model.Chat, error) {
	if queued == nil || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	current, err := s.Chat.GetChat(ctx, queued.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reload broadcast recipient %d: %w", queued.ID, err)
	}
	if current == nil || current.PeerID != queued.PeerID {
		return nil, nil
	}
	if kind == model.BMass {
		return current, nil
	}
	if current.GroupName == nil || queued.GroupName == nil || *current.GroupName != *queued.GroupName {
		return nil, nil
	}
	switch kind {
	case model.BDaily:
		if current.DailySendingTime == nil || queued.DailySendingTime == nil || *current.DailySendingTime != *queued.DailySendingTime {
			return nil, nil
		}
	case model.BPair:
		if !current.PairSending {
			return nil, nil
		}
	case model.BChange:
		if !current.ChangeAlert {
			return nil, nil
		}
	}
	return current, nil
}

func (s *BroadcastService) sendDaily(ctx context.Context, taskID int64, schedules []*model.ScheduleData, grouped map[model.GroupName][]*model.Chat) error {
	var errs []error
	for _, schedule := range schedules {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		if schedule == nil || schedule.Config.Group == nil {
			continue
		}
		images := make(map[bool]*broadcastImage)
		imageErrors := make(map[bool]error)
		for _, queued := range grouped[schedule.Config.Group.GroupName] {
			chat, checkErr := s.currentRecipient(ctx, queued, model.BDaily)
			if checkErr != nil {
				errs = append(errs, checkErr)
				continue
			}
			if chat == nil {
				continue
			}
			img, prepared := images[chat.DarkMode]
			if !prepared {
				img, imageErrors[chat.DarkMode] = s.prepareImage(ctx, schedule, chat.DarkMode)
				images[chat.DarkMode] = img
			}
			err := imageErrors[chat.DarkMode]
			if err == nil {
				chat, err = s.currentRecipient(ctx, queued, model.BDaily)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				if chat == nil || chat.DarkMode != img.schedule.Config.IsDark {
					continue
				}
				err = s.sendSchedule(ctx, chat, img)
			}
			s.recordSend(ctx, taskID, chat, err)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *BroadcastService) schedulePairNotification(ctx context.Context) error {
	// Fifteen minutes before each college lesson, Monday through Saturday.
	for _, t := range [][2]int{{7, 45}, {9, 30}, {11, 15}, {13, 30}, {15, 15}, {17, 0}, {18, 45}} {
		if _, err := s.cron.AddFunc(fmt.Sprintf("0 %d %d * * 1-6", t[1], t[0]), func() {
			s.runJob(func() { s.handlePairNotification(ctx, time.Now()) })
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *BroadcastService) handlePairNotification(ctx context.Context, t time.Time) {
	if !s.canBroadcast(ctx) {
		return
	}
	start := time.Now()
	chats, err := s.Chat.GetChatsWithPairNotification(ctx)
	if err != nil {
		s.reportError(err, "Failed to get chats for pair notifications")
		return
	}
	grouped, groups, confs, invalid, stop := s.prepareBroadcast(ctx, chats)
	s.notifyAndResetInvalidChats(ctx, invalid)
	if stop {
		return
	}
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		s.reportError(err, "Failed to get some schedules for pair notifications")
	}
	task := model.BroadcastTaskLog{Kind: model.BPair, Groups: len(groups)}
	if err := s.Stats.LogBroadcastTask(ctx, &task); err != nil {
		s.reportError(err, "Failed to log pair broadcast task")
		return
	}
	if err := s.sendPairNotificatins(ctx, task.ID, schedules, grouped, groups, t.Add(15*time.Minute)); err != nil {
		s.reportError(err, "Failed to send pair notifications")
	}
	s.finishTask(ctx, &task, start)
}

func (s *BroadcastService) sendPairNotificatins(ctx context.Context, taskID int64, schedules []*model.ScheduleData, grouped map[model.GroupName][]*model.Chat, _ []model.GroupName, t time.Time) error {
	var errs []error
	success := 0
	for _, schedule := range schedules {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		if schedule == nil || schedule.Config.Group == nil {
			continue
		}
		var today *model.ScheduleDay
		for i := range schedule.Days {
			if schedule.Days[i].Date == t.Format("02.01.2006") || schedule.Days[i].Date == t.Format("2006-01-02") {
				today = &schedule.Days[i]
				break
			}
		}
		if today == nil {
			continue
		}
		pair, err := today.CurrentPair(t)
		if errors.Is(err, model.ErrAllPairsPassed) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		switch pair.Kind {
		case model.PairKindEmpty, model.PairKindEvent, model.PairKindIGA, model.PairKindVacation, model.PairKindPractice:
			continue
		}
		text := fmt.Sprintf("Следующая пара в %s, кабинет %s:\n%s\n%s", pair.StartTime, pair.Classroom, pair.Discipline, pair.Teacher)
		for _, queued := range grouped[schedule.Config.Group.GroupName] {
			chat, checkErr := s.currentRecipient(ctx, queued, model.BPair)
			if checkErr != nil {
				errs = append(errs, checkErr)
				continue
			}
			if chat == nil {
				continue
			}
			err := s.Messenger.SendMessagePeer(ctx, int64(chat.PeerID), text)
			s.recordSend(ctx, taskID, chat, err)
			if err != nil {
				errs = append(errs, err)
			} else {
				success++
			}
		}
	}
	log.Info().Int("success", success).Msg("Pair notifications sent")
	return errors.Join(errs...)
}

func (s *BroadcastService) runChangeNotifier(ctx context.Context) {
	interval := viper.GetDuration(config.KeyUpdateMonitorInterval)
	if interval <= 0 {
		interval = time.Minute
	}
	if !sleepContext(ctx, interval/2) {
		return
	}
	for ctx.Err() == nil {
		if s.canBroadcast(ctx) {
			s.handleChangeAlert(ctx)
		}
		if !sleepContext(ctx, interval) {
			return
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}

func (s *BroadcastService) handleChangeAlert(ctx context.Context) {
	if ctx.Err() != nil || s.Messenger == nil {
		return
	}
	start := time.Now()
	chats, err := s.Chat.GetChatsWithChangeAlert(ctx)
	if err != nil {
		s.reportError(err, "Failed to get chats for change alerts")
		return
	}
	grouped, groups, confs, invalid, stop := s.prepareBroadcast(ctx, chats)
	s.notifyAndResetInvalidChats(ctx, invalid)
	if stop {
		return
	}
	changes, err := s.Schedule.GetChanges(ctx, groups)
	if err != nil {
		s.reportError(err, "Failed to get some schedule changes")
	}
	if len(changes) == 0 {
		return
	}
	changedConfigs := make([]model.ScheduleConfig, 0, len(changes))
	for _, conf := range confs {
		if change := changes[conf.Group.GroupName]; change != nil {
			changedConfigs = append(changedConfigs, conf)
		}
	}
	schedules, err := s.Schedule.GetSchedules(ctx, changedConfigs)
	if err != nil {
		s.reportError(err, "Failed to get some schedules for change alerts")
	}
	task := model.BroadcastTaskLog{Kind: model.BChange, Groups: len(changedConfigs)}
	if err := s.Stats.LogBroadcastTask(ctx, &task); err != nil {
		s.reportError(err, "Failed to log change broadcast task")
		return
	}
	if _, err := s.sendChangeReports(ctx, task.ID, schedules, grouped, changes); err != nil {
		s.reportError(err, "Errors while sending change alerts")
	}
	s.finishTask(ctx, &task, start)
}

func (s *BroadcastService) sendChangeReports(ctx context.Context, taskID int64, schedules []*model.ScheduleData, grouped map[model.GroupName][]*model.Chat, changes map[model.GroupName]*model.ScheduleChange) (int, error) {
	var errs []error
	success := 0
	for _, schedule := range schedules {
		if ctx.Err() != nil {
			return success, errors.Join(append(errs, ctx.Err())...)
		}
		if schedule == nil || schedule.Config.Group == nil {
			continue
		}
		change := changes[schedule.Config.Group.GroupName]
		if change == nil {
			continue
		}
		images := make(map[bool]*broadcastImage)
		imageErrors := make(map[bool]error)
		text := change.HTML()
		for _, queued := range grouped[schedule.Config.Group.GroupName] {
			chat, checkErr := s.currentRecipient(ctx, queued, model.BChange)
			if checkErr != nil {
				errs = append(errs, checkErr)
				continue
			}
			if chat == nil {
				continue
			}
			img, prepared := images[chat.DarkMode]
			if !prepared {
				img, imageErrors[chat.DarkMode] = s.prepareImage(ctx, schedule, chat.DarkMode)
				images[chat.DarkMode] = img
			}
			err := imageErrors[chat.DarkMode]
			if err == nil {
				chat, err = s.currentRecipient(ctx, queued, model.BChange)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				if chat == nil || chat.DarkMode != img.schedule.Config.IsDark {
					continue
				}
				err = s.sendSchedule(ctx, chat, img)
			}
			// Permanent refusal stops further sends to the same recipient.
			if !s.Messenger.IsForbidden(err) {
				current, checkErr := s.currentRecipient(ctx, queued, model.BChange)
				if checkErr != nil {
					err = errors.Join(err, checkErr)
				} else if current != nil {
					err = errors.Join(err, s.Messenger.SendMessagePeer(ctx, int64(current.PeerID), text))
				}
			}
			s.recordSend(ctx, taskID, chat, err)
			if err != nil {
				errs = append(errs, err)
			} else {
				success++
			}
		}
	}
	return success, errors.Join(errs...)
}

func (s *BroadcastService) recordSend(ctx context.Context, taskID int64, chat *model.Chat, err error) {
	if s.Messenger.IsForbidden(err) {
		s.handleForbidden(ctx, err, chat)
	}
	s.logBroadcast(ctx, taskID, chat, err)
}

func (s *BroadcastService) logBroadcast(ctx context.Context, taskID int64, chat *model.Chat, sendErr error) {
	logCtx, cancel := broadcastLogContext(ctx)
	defer cancel()
	var errValue *string
	if sendErr != nil {
		value := sendErr.Error()
		errValue = &value
	}
	if err := s.Stats.LogBroadcast(logCtx, model.BroadcastLog{
		TaskID: taskID, ChatID: chat.ID,
		Group: refutil.DerefOrTypeDefault(chat.GroupName), Error: errValue,
	}); err != nil {
		s.reportError(err, "Failed to log broadcast delivery")
	}
}

func (s *BroadcastService) finishTask(ctx context.Context, task *model.BroadcastTaskLog, start time.Time) {
	logCtx, cancel := broadcastLogContext(ctx)
	defer cancel()
	task.Elapsed = time.Since(start).Milliseconds()
	if err := s.Stats.UpdateBroadcastTaskLog(logCtx, task); err != nil {
		s.reportError(err, "Failed to update broadcast task log")
	}
}

// broadcastLogContext preserves delivery outcomes during graceful shutdown,
// with a bounded database write lifetime. Cancellation must stop messenger
// requests, not erase their audit log.
func broadcastLogContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() != nil {
		return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	return ctx, func() {}
}

// BroadcastText starts an explicitly requested administrator broadcast. The
// given HTML text is delivered to every recipient through the messenger, which
// renders it for its platform.
func (s *BroadcastService) BroadcastText(ctx context.Context, chats []*model.Chat, htmlText string) error {
	if s.Messenger == nil {
		return errors.New("messenger is not initialized")
	}
	if s.Services == nil || s.Stats == nil || s.Chat == nil {
		return errors.New("broadcast services are not initialized")
	}
	if len(chats) == 0 {
		return nil
	}
	taskCtx, cancel := context.WithCancel(s.ctx)
	var parentStop func() bool
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			cancel()
			return err
		}
		parentStop = context.AfterFunc(ctx, cancel)
	}
	cleanup := func() {
		cancel()
		if parentStop != nil {
			parentStop()
		}
	}
	// Reserve before logging, so Stop cannot return between setup and sending.
	if !s.jobs.add() {
		cleanup()
		return errors.New("broadcast service has stopped")
	}
	task := model.BroadcastTaskLog{Kind: model.BMass, Groups: 1}
	if err := s.Stats.LogBroadcastTask(taskCtx, &task); err != nil {
		s.jobs.done()
		cleanup()
		return err
	}
	// Callers may reuse the input slice after this asynchronous method returns.
	recipients := make([]*model.Chat, 0, len(chats))
	for _, chat := range chats {
		if chat != nil {
			copy := *chat
			recipients = append(recipients, &copy)
		}
	}
	start := time.Now()
	go func() {
		defer s.jobs.done()
		defer cleanup()
		success := 0
		for _, queued := range recipients {
			if taskCtx.Err() != nil {
				break
			}
			chat, checkErr := s.currentRecipient(taskCtx, queued, model.BMass)
			if checkErr != nil {
				log.Error().Err(checkErr).Msg("Broadcast recipient recheck failed; skipping send")
				continue
			}
			if chat == nil {
				continue
			}
			err := s.Messenger.SendMessagePeer(taskCtx, int64(chat.PeerID), htmlText)
			s.recordSend(taskCtx, task.ID, chat, err)
			if err != nil {
				log.Error().Err(err).Int64("peerID", int64(chat.PeerID)).Msg("Broadcast send failed")
			} else {
				success++
			}
		}
		s.finishTask(taskCtx, &task, start)
		log.Info().Int("success", success).Int("total", len(recipients)).Msg("Mass broadcast finished")
	}()
	return nil
}

func (s *BroadcastService) prepareBroadcast(ctx context.Context, chats []*model.Chat) (map[model.GroupName][]*model.Chat, []model.GroupName, []model.ScheduleConfig, map[model.GroupName][]*model.Chat, bool) {
	grouped := groupChats(chats)
	groups := make([]model.GroupName, 0, len(grouped))
	confs := make([]model.ScheduleConfig, 0, len(grouped))
	invalid := make(map[model.GroupName][]*model.Chat)
	for name := range grouped {
		group, err := s.Schedule.GetGroupByName(ctx, name)
		if err != nil {
			if errors.Is(err, apiclient.ErrNotFound) {
				invalid[name] = grouped[name]
			}
			log.Warn().Err(err).Str("group", string(name)).Msg("Cannot prepare broadcast group")
			continue
		}
		if group == nil {
			continue
		}
		groups = append(groups, name)
		confs = append(confs, model.GroupScheduleConfig(group, false))
	}
	return grouped, groups, confs, invalid, len(confs) == 0
}

func (s *BroadcastService) notifyAndResetInvalidChats(ctx context.Context, invalid map[model.GroupName][]*model.Chat) {
	for group, chats := range invalid {
		for _, queued := range chats {
			chat, checkErr := s.currentRecipient(ctx, queued, model.BAny)
			if checkErr != nil {
				s.reportError(checkErr, "Failed to recheck removed-group recipient")
				continue
			}
			if chat == nil {
				continue
			}
			if chat.DailySendingTime == nil && !chat.PairSending && !chat.ChangeAlert {
				continue
			}
			text := fmt.Sprintf("Группа %s больше не существует на сайте колледжа.\nНастройки сброшены. Выберите новую группу через /settings.", group)
			err := s.Messenger.SendMessagePeer(ctx, int64(chat.PeerID), text)
			if s.Messenger.IsForbidden(err) {
				s.handleForbidden(ctx, err, chat)
			} else if err != nil {
				s.reportError(err, "Failed to notify about removed group")
			}
			chat, checkErr = s.currentRecipient(ctx, queued, model.BAny)
			if checkErr != nil {
				s.reportError(checkErr, "Failed to recheck removed-group reset")
				continue
			}
			if chat == nil {
				continue
			}
			if err := s.Chat.ResetGroupSettings(ctx, chat); err != nil {
				s.reportError(err, "Failed to reset removed group settings")
			}
		}
	}
}

func groupChats(chats []*model.Chat) map[model.GroupName][]*model.Chat {
	grouped := make(map[model.GroupName][]*model.Chat)
	for _, chat := range chats {
		if chat == nil || chat.GroupName == nil {
			continue
		}
		grouped[*chat.GroupName] = append(grouped[*chat.GroupName], chat)
	}
	return grouped
}

func (s *BroadcastService) Stop(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	s.stopped = true
	s.jobs.close()
	s.cancel()
	if s.parentStop != nil {
		s.parentStop()
	}
	cronDone := s.cron.Stop()
	s.mu.Unlock()
	select {
	case <-cronDone.Done():
	case <-ctx.Done():
		return
	}
	s.jobs.wait(ctx)
}

func (s *BroadcastService) handleForbidden(ctx context.Context, sendErr error, chat *model.Chat) {
	current, err := s.currentRecipient(ctx, chat, model.BMass)
	if err != nil {
		s.reportError(err, "Failed to recheck unavailable recipient")
		return
	}
	if current == nil {
		return
	}
	chat = current
	log.Warn().Err(sendErr).Int64("peerID", int64(chat.PeerID)).Msg("Messaging unavailable; disabling subscriptions")
	chat.DailySendingTime = nil
	chat.PairSending = false
	chat.ChangeAlert = false
	if err := s.Chat.UpdateChat(ctx, chat); err != nil {
		s.reportError(err, "Failed to disable unavailable recipient subscriptions")
	}
}

func (s *BroadcastService) reportError(err error, message string) {
	if s.Reporter != nil {
		s.Report().Err(err).Msg(message)
	} else {
		log.Error().Err(err).Msg(message)
	}
}
