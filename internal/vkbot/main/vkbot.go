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
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
	"github.com/rs/zerolog/log"
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
	GetRecentTeachers(context.Context, int64) ([]*model.RecentTeacher, error)
	AddChatRecentTeacher(context.Context, *model.RecentTeacher) error
}

type scheduleService interface {
	GetDepartments(context.Context) ([]model.Department, error)
	GetGroupsByDepartmentName(context.Context, string) ([]model.Group, error)
	GetGroupByName(context.Context, model.GroupName) (*model.Group, error)
	GetTeacherByNameOrID(context.Context, string) (*model.Teacher, error)
	FindTeachersByName(context.Context, string) ([]model.Teacher, error)
	GetSchedule(context.Context, model.ScheduleConfig) (*model.ScheduleData, error)
	PrepareScheduleImage(context.Context, *model.ScheduleData) (string, []byte, error)
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
	now       func() time.Time
	mu        sync.Mutex
	sessions  map[sessionKey]session
}

// New builds the bot around a service bundle. Stats are optional and
// statistics collection is skipped when nil.
func New(api BotAPI, services *service.Services) *Bot {
	b := &Bot{
		api:       api,
		chats:     services.Chat,
		schedules: services.Schedule,
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
		{vkbotutil.TextButton("Сегодня", "today"), vkbotutil.TextButton("Завтра", "tomorrow")},
		{vkbotutil.TextButton("Неделя", "week"), vkbotutil.TextButton("Преподаватель", "teacher")},
		{vkbotutil.TextButton("Настройки", "settings"), vkbotutil.TextButton("Помощь", "help")},
	}}
}

func (b *Bot) send(ctx context.Context, peerID int64, text string, keyboard *vkbotutil.Keyboard) error {
	_, err := b.api.SendMessage(ctx, peerID, text, keyboard)
	return err
}

// Handle processes an incoming message_new event. VK's transport orders
// messages within a peer; sessions additionally belong to the initiating user.
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
			return b.fail(ctx, msg, err)
		}
	} else if err != nil {
		return b.fail(ctx, msg, err)
	}
	started := b.now()
	entry := model.UpdateLog{ChatID: chat.ID, Kind: "message", MessageID: msg.ID, Data: input}
	if msg.Payload != "" {
		entry.Kind = "keyboard"
		entry.Data = msg.Payload
	}
	defer func() {
		if b.stats == nil || command == "stop" {
			return
		}
		entry.Elapsed = int(b.now().Sub(started).Milliseconds())
		if result != nil {
			text := result.Error()
			entry.Error = &text
		}
		if err := b.stats.LogUpdate(ctx, entry); err != nil {
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
	// Keep last activity current only after permission checks.
	chat.UpdatedAt = b.now()
	if command != "stop" {
		if err := b.chats.UpdateChat(ctx, chat); err != nil {
			return b.fail(ctx, msg, err)
		}
	}
	switch command {
	case "start":
		b.clearSession(msg)
		return b.send(ctx, msg.PeerID, "Привет! Я помогу с расписанием группы и преподавателей. Выберите группу через «Настройки → Выбрать группу», затем запрашивайте расписание кнопками ниже.\n\nМеня также можно добавить в беседу ВК. Все команды — /help.", mainKeyboard())
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
		return b.send(ctx, msg.PeerID, "Настройки и история выбора преподавателей удалены. Рассылки остановлены. Чтобы настроить бота заново, отправьте /start.", &vkbotutil.Keyboard{Buttons: [][]vkbotutil.Button{}})
	case "settings":
		b.clearSession(msg)
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
		return b.groupSchedule(ctx, chat, msg, command, arg, &entry)
	case "week", "day":
		return b.keyboardSchedule(ctx, chat, msg, command, arg, &entry)
	case "teacher":
		return b.teacherSearch(ctx, chat, msg, arg, &entry)
	case "teachers":
		return b.teacherPage(ctx, chat, msg, arg)
	case "showteacher":
		return b.teacherSchedule(ctx, chat, msg, arg, &entry)
	default:
		if _, err := model.GroupName(input).ValidateFormat(); err == nil {
			return b.groupSchedule(ctx, chat, msg, "week", input, &entry)
		}
		if chat.IsPrivate() {
			return b.send(ctx, msg.PeerID, "Выберите команду на клавиатуре или отправьте /help.", mainKeyboard())
		}
		return nil
	}
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
		"завтра": "tomorrow", "неделя": "week", "преподаватель": "teacher",
		"настройки": "settings", "отмена": "cancel",
	}
	if alias, ok := aliases[command]; ok {
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
