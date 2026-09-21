package vkbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
)

func onOff(value bool) string {
	if value {
		return "вкл."
	}
	return "выкл."
}

func (b *Bot) settings(ctx context.Context, chat *model.Chat) error {
	group := "не выбрана"
	if chat.GroupName != nil {
		group = string(*chat.GroupName)
	}
	daily := "выключено"
	if chat.DailySendingTime != nil {
		daily = *chat.DailySendingTime
	}
	text := fmt.Sprintf("Настройки\n\nГруппа: %s\nЕжедневное расписание: %s\nНапоминания о парах: %s\nИзменения расписания: %s\nТёмная тема: %s",
		group, daily, onOff(chat.PairSending), onOff(chat.ChangeAlert), onOff(chat.DarkMode))
	rows := [][]vkbotutil.Button{
		{vkbotutil.TextButton("Выбрать группу", "departments\n0")},
		{vkbotutil.TextButton("Ежедневная рассылка", "daily")},
		{vkbotutil.TextButton("Пары: "+onOff(chat.PairSending), "pair")},
		{vkbotutil.TextButton("Изменения: "+onOff(chat.ChangeAlert), "change")},
		{vkbotutil.TextButton("Тёмная тема: "+onOff(chat.DarkMode), "dark")},
	}
	if !chat.IsPrivate() {
		rows = append(rows, []vkbotutil.Button{vkbotutil.TextButton("Права участников", "access")})
	}
	return b.send(ctx, int64(chat.PeerID), text, &vkbotutil.Keyboard{Inline: true, Buttons: rows})
}

func (b *Bot) departments(ctx context.Context, chat *model.Chat, msg vkclient.Message, page int) error {
	departments, err := b.schedules.GetDepartments(ctx)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if len(departments) == 0 {
		return b.send(ctx, msg.PeerID, "Отделения пока не опубликованы. Попробуйте позже.", nil)
	}
	items := make([]vkbotutil.MenuItem, 0, len(departments))
	for _, department := range departments {
		items = append(items, vkbotutil.MenuItem{Label: department.Name, Command: "groups\n" + department.ID + "\n0"})
	}
	b.setSession(msg, "group", "")
	return b.send(ctx, msg.PeerID, "Выберите отделение или отправьте название группы сообщением.", vkbotutil.PagedKeyboard(items, page, "departments", "settings"))
}

func (b *Bot) groups(ctx context.Context, chat *model.Chat, msg vkclient.Message, arg string) error {
	parts := strings.SplitN(arg, "\n", 2)
	departmentID := parts[0]
	page := 0
	if len(parts) == 2 {
		page = pageNumber(parts[1])
	}
	departments, err := b.schedules.GetDepartments(ctx)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	departmentName := ""
	for _, department := range departments {
		if department.ID == departmentID {
			departmentName = department.Name
			break
		}
	}
	if departmentName == "" {
		return b.send(ctx, msg.PeerID, "Отделение больше не найдено. Откройте выбор группы заново.", nil)
	}
	groups, err := b.schedules.GetGroupsByDepartmentName(ctx, departmentName)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if len(groups) == 0 {
		return b.send(ctx, msg.PeerID, "В этом отделении пока нет доступных групп.", nil)
	}
	items := make([]vkbotutil.MenuItem, 0, len(groups))
	for _, group := range groups {
		items = append(items, vkbotutil.MenuItem{Label: string(group.GroupName), Command: "setgroup\n" + string(group.GroupName)})
	}
	b.setSession(msg, "group", departmentName)
	return b.send(ctx, msg.PeerID, "Отделение: "+departmentName+"\nВыберите группу или отправьте её название.", vkbotutil.PagedKeyboard(items, page, "groups\n"+departmentID, "departments\n0"))
}

func (b *Bot) setGroup(ctx context.Context, chat *model.Chat, msg vkclient.Message, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return b.departments(ctx, chat, msg, 0)
	}
	if _, err := model.GroupName(name).ValidateFormat(); err != nil {
		return b.send(ctx, msg.PeerID, "Введите название группы, например «ИСПт-22-(9)-2» или «испт 22 9 2». Отмена — /cancel.", nil)
	}
	group, err := b.schedules.GetGroupByName(ctx, model.GroupName(name))
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if group == nil {
		return b.send(ctx, msg.PeerID, "Группа не найдена. Проверьте название.", nil)
	}
	chat.GroupName, chat.DepartmentName = &group.GroupName, &group.DepartmentName
	chat.State = model.ChatStateDefault
	if err := b.chats.UpdateChat(ctx, chat); err != nil {
		return b.fail(ctx, msg, err)
	}
	b.clearSession(msg)
	if err := b.send(ctx, msg.PeerID, "Выбрана группа "+string(group.GroupName)+". Расписание доступно кнопками ниже.", mainKeyboard()); err != nil {
		return err
	}
	return b.settings(ctx, chat)
}

func (b *Bot) daily(ctx context.Context, chat *model.Chat, msg vkclient.Message, arg string) error {
	arg = strings.TrimSpace(strings.ToLower(arg))
	if arg == "off" || arg == "выкл" || arg == "выключить" {
		chat.DailySendingTime = nil
	} else {
		if chat.GroupName == nil {
			return b.send(ctx, msg.PeerID, "Сначала выберите группу в /settings.", nil)
		}
		if arg == "" {
			b.setSession(msg, "time", "")
			return b.send(ctx, msg.PeerID, "Отправьте время ежедневного расписания в формате ЧЧ:ММ, например 07:30. Время — Екатеринбург (UTC+5).\nОтмена — /cancel.", &vkbotutil.Keyboard{Inline: true, Buttons: [][]vkbotutil.Button{{vkbotutil.TextButton("Выключить рассылку", "daily\noff")}, {vkbotutil.TextButton("Отмена", "cancel")}}})
		}
		parsed, err := time.Parse("15:04", arg)
		if err != nil || len(arg) != 5 || parsed.Format("15:04") != arg {
			return b.send(ctx, msg.PeerID, "Неверное время. Используйте ЧЧ:ММ от 00:00 до 23:59, например 07:30.", nil)
		}
		chat.DailySendingTime = &arg
	}
	chat.State = model.ChatStateDefault
	if err := b.chats.UpdateChat(ctx, chat); err != nil {
		return b.fail(ctx, msg, err)
	}
	b.clearSession(msg)
	return b.settings(ctx, chat)
}

func (b *Bot) toggle(ctx context.Context, chat *model.Chat, msg vkclient.Message, kind string) error {
	if kind != "dark" && chat.GroupName == nil {
		return b.send(ctx, msg.PeerID, "Сначала выберите группу в /settings.", nil)
	}
	switch kind {
	case "pair":
		chat.PairSending = !chat.PairSending
	case "change":
		chat.ChangeAlert = !chat.ChangeAlert
	case "dark":
		chat.DarkMode = !chat.DarkMode
	}
	if err := b.chats.UpdateChat(ctx, chat); err != nil {
		return b.fail(ctx, msg, err)
	}
	return b.settings(ctx, chat)
}

func (b *Bot) accessMenu(ctx context.Context, chat *model.Chat) error {
	if chat.IsPrivate() {
		return b.send(ctx, int64(chat.PeerID), "Уровни доступа настраиваются только в беседе. В личных сообщениях доступны все команды.", nil)
	}
	levels := []string{"Все команды всем", "Настройки администраторам", "Всё администраторам"}
	rows := make([][]vkbotutil.Button, 0, 4)
	for i, label := range levels {
		if chat.Access == model.ChatAccessLevel(i) {
			label = "✓ " + label
		}
		rows = append(rows, []vkbotutil.Button{vkbotutil.TextButton(label, "setaccess\n"+strconv.Itoa(i))})
	}
	rows = append(rows, []vkbotutil.Button{vkbotutil.TextButton("Назад", "settings")})
	return b.send(ctx, int64(chat.PeerID), "Права участников беседы\n\nВыберите, кто может запрашивать расписание и изменять настройки. Менять сам уровень доступа всегда могут только администраторы беседы.", &vkbotutil.Keyboard{Inline: true, Buttons: rows})
}

func (b *Bot) setAccess(ctx context.Context, chat *model.Chat, msg vkclient.Message, arg string) error {
	if chat.IsPrivate() {
		return b.accessMenu(ctx, chat)
	}
	level, err := strconv.Atoi(arg)
	if err != nil || level < 0 || level > 2 {
		return b.send(ctx, msg.PeerID, "Неизвестный уровень доступа. Откройте /access заново.", nil)
	}
	chat.Access = model.ChatAccessLevel(level)
	if err := b.chats.UpdateChat(ctx, chat); err != nil {
		return b.fail(ctx, msg, err)
	}
	return b.accessMenu(ctx, chat)
}

func pageNumber(text string) int {
	page, err := strconv.Atoi(text)
	if err != nil || page < 0 {
		return 0
	}
	return page
}
