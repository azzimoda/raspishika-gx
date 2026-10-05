// Package vkbot implements the Russian schedule assistant for VK communities:
// message routing, group and teacher schedule views, and per-chat settings. It
// mirrors the Telegram bot and shares its model and service interfaces.
package vkbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/service"
	vkclient "github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// BotAPI is the transport surface the bot needs: message posting with an
// optional keyboard, photo sending, and conversation admin checks.
type BotAPI interface {
	SendMessage(context.Context, int64, string, *vkbotutil.Keyboard) (int, error)
	SendPhoto(context.Context, int64, string, []byte, string, *vkbotutil.Keyboard) (int, error)
	IsAdmin(context.Context, int64, int64) (bool, error)
}

type chatService interface {
	GetChatByChatID(context.Context, model.ChatID) (*model.Chat, error)
	CreateChat(context.Context, *model.Chat) error
	UpdateChat(context.Context, *model.Chat) error
	DeleteChat(context.Context, int64) error
	ResetGroupSettings(context.Context, *model.Chat) error
	GetRecentTeachers(context.Context, int64) ([]*model.RecentTeacher, error)
	AddChatRecentTeacher(context.Context, *model.RecentTeacher) error
	CountAllChats(context.Context) (int, error)
}

type scheduleService interface {
	GetDepartments(context.Context) ([]model.Department, error)
	GetGroupsByDepartmentName(context.Context, string) ([]model.Group, error)
	GetGroupByName(context.Context, model.GroupName) (*model.Group, error)
	GetTeacherByNameOrID(context.Context, string) (*model.Teacher, error)
	FindTeachersByName(context.Context, string) ([]model.Teacher, error)
	GetSchedule(context.Context, model.ScheduleConfig) (*model.ScheduleData, error)
	PrepareScheduleImage(context.Context, *model.ScheduleData) (string, []byte, error)
	IsVacation(context.Context) (bool, error)
}

type statsService interface {
	LogUpdate(context.Context, model.UpdateLog) error
}

type sessionKey struct{ peerID, userID int64 }

type session struct {
	kind    string
	query   string
	expires time.Time
}

// Bot routes incoming VK messages. One instance is shared by the whole
// community; sessions are scoped to the initiating user in a peer.
type Bot struct {
	api       BotAPI
	chats     chatService
	schedules scheduleService
	stats     statsService
	reporter  reporter.Reporter
	now       func() time.Time
	mu        sync.Mutex
	sessions  map[sessionKey]session
}

// New builds the bot around a service bundle. Stats and reporter are optional;
// when reporter is nil, handler reports fall back to log-only.
func New(api BotAPI, services *service.Services, report reporter.Reporter) *Bot {
	b := &Bot{
		api:       api,
		chats:     services.Chat,
		schedules: services.Schedule,
		reporter:  report,
		now:       time.Now,
		sessions:  make(map[sessionKey]session),
	}
	if services.Stats != nil {
		b.stats = services.Stats
	}
	return b
}

const helpText = `Расписание во ВКонтакте:
/today — сегодня
/tomorrow — завтра
/week — неделя в картинке
/teacher Фамилия — поиск преподавателя
/settings — группа, рассылки и оформление
/access — права участников беседы
/cancel — отменить ввод
/stop — удалить настройки и остановить рассылки

Можно пользоваться кнопками или прислать название любой группы, например «испт 22 9 2». Команды /today, /tomorrow и /week также принимают название группы после команды.

В настройках доступны ежедневное расписание, напоминания за 15 минут до пар и уведомления об изменениях. В беседе настройки изначально доступны только администраторам. Для проверки прав выдайте боту права администратора беседы.`

func mainKeyboard() *vkbotutil.Keyboard {
	return &vkbotutil.Keyboard{Buttons: [][]vkbotutil.Button{
		{vkbotutil.TextButton("Моё расписание", "week")},
		{vkbotutil.TextButton("Настройки", "settings"), vkbotutil.TextButton("Преподаватель", "teacher")},
	}}
}

func (b *Bot) send(ctx context.Context, peerID int64, text string, keyboard *vkbotutil.Keyboard) error {
	_, err := b.api.SendMessage(ctx, peerID, text, keyboard)
	return err
}

// Handle processes an incoming message_new event. VK's transport orders
// messages within a peer; sessions additionally belong to the initiating user.
// Every processed update is written to update_logs the same way the Telegram
// bot does: metadata and errors flow through the context, the default handler
// opts out, and the error column always carries a pointer (empty on success).
func (b *Bot) Handle(ctx context.Context, msg vkclient.Message) (result error) {
	if msg.Out || msg.FromID <= 0 || msg.PeerID <= 0 {
		return nil
	}
	command, arg, recognized := parseCommand(msg)
	input := strings.TrimSpace(msg.Text)
	if input == "" && !recognized {
		return nil
	}
	state := b.getSession(msg)
	if msg.PeerID >= vkclient.ChatPeerOffset && !recognized && state.kind == "" {
		if _, err := model.GroupName(input).ValidateFormat(); err != nil {
			return nil
		}
	}
	chat, err := b.chats.GetChatByChatID(ctx, model.ChatID(msg.PeerID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		chat = &model.Chat{PeerID: model.ChatID(msg.PeerID), State: model.ChatStateDefault}
		if !chat.IsPrivate() {
			chat.Access = model.ChatAccessConfigAdmin
		}
		if err = b.chats.CreateChat(ctx, chat); err != nil {
			b.reportChat(nil, msg).Err(err).Msg("Failed to create or update chat")
			_ = b.fail(ctx, msg, err)
			return nil
		}
		if b.reporter != nil {
			go b.sendNewChatReport(chat, msg)
		}
	} else if err != nil {
		b.reportChat(nil, msg).Err(err).Msg("Failed to create or update chat")
		_ = b.fail(ctx, msg, err)
		return nil
	}
	log.Trace().Any("message", msg).Msg("Received VK update")
	started := b.now()

	var handlerErrs []error
	ctx = context.WithValue(ctx, keyError, &handlerErrs)
	var noLogFlag bool
	ctx = context.WithValue(ctx, keyNoLogFlag, &noLogFlag)
	var groupOrTeacher string
	ctx = context.WithValue(ctx, keyGroupOrTeacher, &groupOrTeacher)
	var cachedSchedule bool
	ctx = context.WithValue(ctx, keyCached, &cachedSchedule)

	updateKind := "message"
	updateData := input
	if msg.Payload != "" {
		updateKind = "keyboard"
		updateData = msg.Payload
	}

	defer func() {
		elapsedTime := b.now().Sub(started)
		log.Trace().Any("message", msg).Msg("VK update processed")

		if noLogFlag {
			log.Trace().Msg("No log flag enabled")
			return
		}

		handlerErr := result
		if len(handlerErrs) > 0 {
			handlerErr = errors.Join(handlerErr, errors.Join(handlerErrs...))
		}
		handlerErrStr := ""
		if handlerErr != nil {
			handlerErrStr = handlerErr.Error()
			if reportable(handlerErr) {
				b.reportChat(chat, msg).Err(handlerErr).
					Debug("update_type", updateKind).Debug("update_data", updateData).
					Msg("Handler error")
			} else {
				log.Debug().Err(handlerErr).Int("message_id", msg.ID).Int64("chat_id", msg.PeerID).Msg("VK handler error")
			}
		}

		logEvent := log.Info().Dur("elapsed_time", elapsedTime)
		if updateKind == "keyboard" {
			logEvent.Int("message_id", msg.ID).Int64("chat_id", msg.PeerID).
				Str("data", updateData).Msg("VK keyboard update handled")
		} else {
			logEvent.Int("message_id", msg.ID).Int64("chat_id", msg.PeerID).
				Str("text", shortenText(updateData, 100)).Msg("VK message handled")
		}

		if b.stats == nil {
			return
		}
		if err := b.stats.LogUpdate(ctx, model.UpdateLog{
			ChatID:         chat.ID,
			Kind:           updateKind,
			MessageID:      msg.ID,
			Data:           updateData,
			GroupOrTeacher: groupOrTeacher,
			IsCached:       cachedSchedule,
			Elapsed:        int(elapsedTime.Milliseconds()),
			Error:          &handlerErrStr,
		}); err != nil {
			log.Warn().Err(err).Msg("VK update statistics failed")
		}
	}()
	if !recognized && state.kind != "" {
		switch state.kind {
		case "group":
			command, arg = "setgroup", input
		case "time":
			command, arg = "daily", input
		case "teacher":
			command, arg = "teacher", input
		}
	}
	if !chat.IsPrivate() && (command == "access" || command == "setaccess") {
		if err := b.requireAdmin(ctx, msg); err != nil {
			return b.permissionReply(ctx, msg, err)
		}
	} else if err := b.requireAccess(ctx, chat, msg, configurationCommand(command)); err != nil {
		return b.permissionReply(ctx, msg, err)
	}
	if command != "stop" {
		if err := b.chats.UpdateChat(ctx, chat); err != nil {
			return b.fail(ctx, msg, err)
		}
	}
	switch command {
	case "start":
		b.clearSession(msg)
		if chat.GroupName == nil || string(*chat.GroupName) == "" {
			b.offerToSetGroupOnStart(ctx, msg, chat)
			return nil
		}
		return b.send(ctx, msg.PeerID, "Привет! Я помогу с расписанием группы и преподавателей. Используйте команды на клавиатуре или /help.", mainKeyboard())
	case "help":
		return b.send(ctx, msg.PeerID, helpText, mainKeyboard())
	case "cancel":
		b.clearSession(msg)
		return b.send(ctx, msg.PeerID, "Ввод отменён.", mainKeyboard())
	case "stop":
		if err := b.chats.DeleteChat(ctx, chat.ID); err != nil {
			return b.fail(ctx, msg, err)
		}
		b.clearPeerSessions(msg.PeerID)
		count, err := b.chats.CountAllChats(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to count all chats")
		}
		b.reportChat(chat, msg).Silent(true).Msgf("User stopped the bot ☹ (×%d rests)", count)
		return b.send(ctx, msg.PeerID, "Настройки и история выбора преподавателей удалены. Рассылки остановлены. Чтобы настроить бота заново, отправьте /start.", &vkbotutil.Keyboard{Buttons: [][]vkbotutil.Button{}})
	case "settings":
		b.clearSession(msg)
		if viper.GetBool(config.KeyHandleVacation) && b.vacationActive(ctx) {
			return b.sendVacationAnswer(ctx, msg.PeerID, true)
		}
		return b.settings(ctx, chat)
	case "departments":
		return b.departments(ctx, chat, msg, pageNumber(arg))
	case "groups":
		return b.groups(ctx, chat, msg, arg)
	case "setgroup":
		return b.setGroup(ctx, chat, msg, arg)
	case "daily":
		return b.daily(ctx, chat, msg, arg)
	case "pair", "change", "dark":
		return b.toggle(ctx, chat, msg, command)
	case "access":
		return b.accessMenu(ctx, chat)
	case "setaccess":
		return b.setAccess(ctx, chat, msg, arg)
	case "today", "tomorrow":
		return b.groupSchedule(ctx, chat, msg, command, arg)
	case "week", "day":
		return b.keyboardSchedule(ctx, chat, msg, command, arg)
	case "teacher":
		return b.teacherSearch(ctx, chat, msg, arg)
	case "teachers":
		return b.teacherPage(ctx, chat, msg, arg)
	case "showteacher":
		return b.teacherSchedule(ctx, chat, msg, arg)
	default:
		if _, err := model.GroupName(input).ValidateFormat(); err == nil {
			return b.groupSchedule(ctx, chat, msg, "week", input)
		}
		setNoLogFlag(ctx)
		log.Debug().Str("text", input).Msg("Unhandled VK message")
		if chat.IsPrivate() {
			return b.send(ctx, msg.PeerID, "Выберите команду на клавиатуре или отправьте /help.", mainKeyboard())
		}
		return nil
	}
}

// reportable returns true for errors worth sending to the admin — i.e. every
// handler error except the two benign cases the bot already handled for the
// user (not-found and scraper unavailable). Matching the Telegram bot avoids
// flooding the admin with typos in group names and vacation windows. A nil
// error is never reportable.
func reportable(err error) bool {
	return err != nil && !errors.Is(err, apiclient.ErrNotFound) && !errors.Is(err, apiclient.ErrServiceUnavailable)
}

func configurationCommand(command string) bool {
	switch command {
	case "settings", "departments", "groups", "setgroup", "daily", "pair", "change", "dark", "stop", "access", "setaccess":
		return true
	}
	return false
}

var errNotAdmin = errors.New("administrator rights required")

func (b *Bot) requireAdmin(ctx context.Context, msg vkclient.Message) error {
	ok, err := b.api.IsAdmin(ctx, msg.PeerID, msg.FromID)
	if err != nil {
		return fmt.Errorf("проверка прав: %w", err)
	}
	if !ok {
		return errNotAdmin
	}
	return nil
}

func (b *Bot) requireAccess(ctx context.Context, chat *model.Chat, msg vkclient.Message, config bool) error {
	if chat.IsPrivate() {
		return nil
	}
	if chat.Access == model.ChatAccessAdminOnly || (config && chat.Access != model.ChatAccessAll) {
		return b.requireAdmin(ctx, msg)
	}
	return nil
}

func (b *Bot) permissionReply(ctx context.Context, msg vkclient.Message, err error) error {
	text := "Эта команда доступна только администраторам беседы."
	if !errors.Is(err, errNotAdmin) {
		text = "Не удалось проверить права. Убедитесь, что бот добавлен в администраторы беседы, и попробуйте снова."
	}
	return b.send(ctx, msg.PeerID, text, nil)
}

func (b *Bot) fail(ctx context.Context, msg vkclient.Message, err error) error {
	text := "Не удалось выполнить запрос. Попробуйте ещё раз немного позже."
	if errors.Is(err, apiclient.ErrServiceUnavailable) {
		text = "Сайт расписания временно недоступен или расписание ещё не опубликовано. Попробуйте позже."
	}
	if errors.Is(err, apiclient.ErrNotFound) {
		text = "Группа или преподаватель не найдены. Проверьте название или выберите их заново."
	}
	return errors.Join(err, b.send(ctx, msg.PeerID, text, nil))
}

func (b *Bot) reportChat(chat *model.Chat, msg vkclient.Message) reporter.ReportBuilder {
	if b.reporter == nil || chat == nil {
		return reporter.EmptyReportBuilder()
	}
	return b.reporter.Report().
		Debug("chatID", chat.PeerID).
		Debug("user_id", msg.FromID).
		Debug("group", refDerefOrEmpty(chat.GroupName))
}

func refDerefOrEmpty[T any](ptr *T) string {
	if ptr == nil {
		return ""
	}
	return fmt.Sprintf("%v", *ptr)
}

func (b *Bot) offerToSetGroupOnStart(ctx context.Context, msg vkclient.Message, chat *model.Chat) {
	err := b.departments(ctx, chat, msg, 0)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send department selection menu")
		b.send(ctx, msg.PeerID, "Не удалось открыть настройки группы. Попробуйте позже.", mainKeyboard())
	}
}

// sendNewChatReport sends a "new chat" report to the admin and then waits
// (up to ~100 s) for the chat to finish configuring its group. Once the group
// and department are set the initial report is replaced with a "finished
// configuration" report, mirroring the Telegram bot's sendNewChatReport.
func (b *Bot) sendNewChatReport(chat *model.Chat, msg vkclient.Message) {
	count, err := b.chats.CountAllChats(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to count all chats")
		count = 0
	}

	report, err := b.reportChat(chat, msg).Silent(true).Msgf("New chat registered (×%d)", count)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send new chat report")
		return
	}

	for range 10 {
		time.Sleep(10 * time.Second)

		updated, err := b.chats.GetChatByChatID(context.Background(), chat.PeerID)
		if err != nil {
			continue
		}
		if updated.GroupName != nil && updated.DepartmentName != nil {
			if _, err := report.DeleteMessage(); err != nil {
				log.Warn().Err(err).Msg("Failed to delete new chat report")
			}
			b.reportChat(updated, msg).Silent(true).Msgf("New chat (×%d) finished configuration: %s — %s", count, *updated.GroupName, *updated.DepartmentName)
			return
		}
	}
}

func (b *Bot) getSession(msg vkclient.Message) session {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := sessionKey{msg.PeerID, msg.FromID}
	s := b.sessions[key]
	if !s.expires.After(b.now()) {
		delete(b.sessions, key)
		return session{}
	}
	return s
}

func (b *Bot) setSession(msg vkclient.Message, kind, query string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	for key, s := range b.sessions {
		if !s.expires.After(now) {
			delete(b.sessions, key)
		}
	}
	b.sessions[sessionKey{msg.PeerID, msg.FromID}] = session{kind: kind, query: query, expires: now.Add(15 * time.Minute)}
}

func (b *Bot) clearSession(msg vkclient.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, sessionKey{msg.PeerID, msg.FromID})
}

func (b *Bot) clearPeerSessions(peerID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key := range b.sessions {
		if key.peerID == peerID {
			delete(b.sessions, key)
		}
	}
}

// parseCommand extracts a command and its argument from a message: either the
// keyboard payload {"command":"..."} or the text itself. Commands may carry an
// optional leading slash and be addressed by a VK mention in conversations.
func parseCommand(msg vkclient.Message) (command, arg string, recognized bool) {
	text := strings.TrimSpace(msg.Text)
	if msg.Payload != "" {
		var payload struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(msg.Payload), &payload) != nil || payload.Command == "" {
			return "", "", false
		}
		text = strings.TrimSpace(payload.Command)
	}
	if strings.HasPrefix(text, "[club") || strings.HasPrefix(text, "[public") {
		if end := strings.Index(text, "]"); end >= 0 {
			text = strings.TrimSpace(strings.TrimLeft(text[end+1:], ", "))
		}
	}
	command = text
	if index := strings.IndexFunc(text, unicode.IsSpace); index >= 0 {
		command, arg = text[:index], strings.TrimSpace(text[index:])
	}
	command = strings.ToLower(strings.TrimPrefix(command, "/"))
	aliases := map[string]string{
		"начать": "start", "начало": "start", "помощь": "help", "сегодня": "today",
		"завтра": "tomorrow", "неделя": "week", "расписание": "week",
		"моё расписание": "week", "мое расписание": "week", "преподаватель": "teacher",
		"настройки": "settings", "отмена": "cancel",
	}
	noArgAliases := map[string]bool{"start": true, "help": true, "cancel": true, "settings": true, "today": true, "tomorrow": true, "week": true}
	if alias, ok := aliases[command]; ok {
		if noArgAliases[alias] && arg != "" {
			return "", "", false
		}
		command = alias
	}
	switch command {
	case "start", "help", "cancel", "stop", "settings", "departments", "groups", "setgroup",
		"daily", "pair", "change", "dark", "access", "setaccess", "today", "tomorrow",
		"week", "day", "teacher", "teachers", "showteacher":
		return command, arg, true
	}
	return "", "", false
}
