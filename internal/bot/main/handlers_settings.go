package mainbot

import (
	"context"
	"fmt"
	"strconv"
	"time"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (h *handler) handleCmdSettings(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling command settings...")

	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID

	_, err := botutil.DeleteMessage(ctx, b, update.Message)
	addHandlerCtxErr(ctx, err)

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            settingsMenuText(chat),
		ReplyMarkup:     settingsMenuMarkup(chat),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled command settings")
}

func (h *handler) handleCQConfigGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling CQ config group...")

	message := update.CallbackQuery.Message.Message

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	_, err := botutil.DeleteMessage(ctx, b, message)
	addHandlerCtxErr(ctx, err)

	err = h.sendDepartmentSelectionMenu(ctx, b, chat, update)
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ config group")
}

func (h *handler) handleCQConfigDailyTime(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling CQ config daily time...")

	message := update.CallbackQuery.Message.Message

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	_, err := botutil.DeleteMessage(ctx, b, message)
	addHandlerCtxErr(ctx, err)

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateSelectingTime)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
	}

	time := "<u>Время не установлено</u>"
	if chat.DailySendingTime != nil {
		time = "Установленное время: <u>" + *chat.DailySendingTime + "</u>"
	}
	text := fmt.Sprintf("%s\n\nПришлите желаемое время рассылки, например <code>19:00</code>", time)
	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Закрыть", CallbackData: botutil.CallbackCommandDelete}},
			},
		},
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ config daily time")
}
func (h *handler) handleTextTime(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling text time...")

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	t, err := time.Parse("15:04", update.Message.Text)
	if err != nil {
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			ParseMode:       models.ParseModeHTML,
			Text:            "Неправильный вормат времени, попробуйте ещё раз",
		})
		return
	}
	timeStr := t.Format("15:04")

	chat.DailySendingTime = &timeStr
	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		ParseMode:       models.ParseModeHTML,
		Text:            fmt.Sprintf("Время рассылки установлено на <u>%s</u>", timeStr),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled text time")
}
func (h *handler) handleCQDailyOff(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling CQ daily off...")

	message := update.CallbackQuery.Message.Message

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	chat.DailySendingTime = nil
	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	addHandlerCtxErr(ctx, updateSettingsMenuMessage(ctx, b, message, chat))

	log.Info().Msg("Handled CQ daily off")
}

func (h *handler) handleCQConfigReminder(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.CallbackQuery.Message.Message
	chatID := message.Chat.ID
	threadID := message.MessageThreadID

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	chat.PairSending = command.Arg(0) == "true"
	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	addHandlerCtxErr(ctx, updateSettingsMenuMessage(ctx, b, message, chat))

	log.Info().Msg("Handled CQ config reminder")
}

func (h *handler) handleCQConfigChange(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.CallbackQuery.Message.Message
	chatID := message.Chat.ID
	threadID := message.MessageThreadID

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	chat.ChangeAlert = command.Arg(0) == "true"
	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	addHandlerCtxErr(ctx, updateSettingsMenuMessage(ctx, b, message, chat))

	log.Info().Msg("Handled CQ config change")
}

func (h *handler) handleCQConfigDarkMode(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.CallbackQuery.Message.Message
	chatID := message.Chat.ID
	threadID := message.MessageThreadID

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	chat.DarkMode = command.Arg(0) == "true"
	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	addHandlerCtxErr(ctx, updateSettingsMenuMessage(ctx, b, message, chat))

	log.Info().Msg("Handled CQ config dark mode")
}

func updateSettingsMenuMessage(ctx context.Context, b *bot.Bot, message *models.Message, chat *model.Chat) error {
	if _, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        settingsMenuText(chat),
		ReplyMarkup: settingsMenuMarkup(chat),
	}); err != nil {
		log.Warn().Msg("Failed to update settings menu message")
		return err
	}
	log.Debug().Msg("Updated settings menu message")
	return nil
}

func settingsMenuText(chat *model.Chat) string {
	dailyTime := "выключено"
	if t := refutil.DerefOrTypeDefault(chat.DailySendingTime); t != "" {
		dailyTime = t
	}
	pairNotification := onOffStr(chat.PairSending)
	changesNotificatin := onOffStr(chat.ChangeAlert)
	theme := "светлая"
	if chat.DarkMode {
		theme = "тёмная"
	}

	text := fmt.Sprintf(`<b>Настройки</b>

Группа: <u>%s</u>
Ежедневная рассылка: <u>%s</u>
Напоминания перед парами: <u>%s</u>
Уведомления об изменениях: <u>%s</u>
Тема: <u>%s</u>`,
		refutil.DerefOrTypeDefault(chat.GroupName),
		dailyTime,
		pairNotification,
		changesNotificatin,
		theme,
	)
	if !chat.IsPrivate() {
		text += fmt.Sprintf("\nУровень доступа: <u>%d</u>", chat.Access)
	}

	return text
}
func settingsMenuMarkup(chat *model.Chat) models.InlineKeyboardMarkup {
	keyboard := make([][]models.InlineKeyboardButton, 0)

	// Student group
	keyboard = append(keyboard, []models.InlineKeyboardButton{{Text: "Изменить группу", CallbackData: botutil.CallbackCommandConfigGroup}})

	// Daily sending
	if chat.DailySendingTime == nil {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Вкл. ежедневную рассылку", CallbackData: botutil.CallbackCommandConfigDailyTime},
		})
	} else {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Изменить время", CallbackData: botutil.CallbackCommandConfigDailyTime},
			{Text: "Выкл. рассылку", CallbackData: botutil.CallbackCommandDailyOff},
		})
	}

	// Pair notification
	if chat.PairSending {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Выкл. напоминания перед парами", CallbackData: botutil.CallbackCommandConfigReminder + "\nfalse"},
		})
	} else {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Вкл. напоминания перед парами", CallbackData: botutil.CallbackCommandConfigReminder + "\ntrue"},
		})
	}

	// Change alerts
	if chat.ChangeAlert {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Выкл. уведомления об изменениях", CallbackData: botutil.CallbackCommandConfigChange + "\nfalse"},
		})
	} else {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Вкл. уведомления об изменениях", CallbackData: botutil.CallbackCommandConfigChange + "\ntrue"},
		})
	}

	// Dark mode
	if chat.DarkMode {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Вкл. светлую тему", CallbackData: botutil.CallbackCommandConfigDarkMode + "\nfalse"},
		})
	} else {
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: "Вкл. тёмную тему", CallbackData: botutil.CallbackCommandConfigDarkMode + "\ntrue"},
		})
	}

	// Group chat access
	if !chat.IsPrivate() {
		row := []models.InlineKeyboardButton{
			{Text: "0", CallbackData: botutil.CallbackCommandSetAccess + "\n0"},
			{Text: "1", CallbackData: botutil.CallbackCommandSetAccess + "\n1"},
			{Text: "2", CallbackData: botutil.CallbackCommandSetAccess + "\n2"},
		}
		for i := range 3 {
			if i == int(chat.Access) {
				row[i].Text = fmt.Sprintf("[%d]", chat.Access)
			}
		}
		keyboard = append(keyboard, row)
	}

	// Close button
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: "Закрыть", CallbackData: botutil.CallbackCommandDeleteConfig},
	})

	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

func (h *handler) handleCQSelectDepartment(ctx context.Context, b *bot.Bot, update *models.Update) {
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	message := update.CallbackQuery.Message.Message

	_, err := botutil.DeleteMessage(ctx, b, message)
	addHandlerCtxErr(ctx, err)

	groups, err := h.Schedule.GetGroupsByDepartmentName(ctx, command.Arg(0))
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateSelectingGroup)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            "Выберите группы на клавиатуре или введите название в верном формате, например: ИСПт-22-(9)-2",
		ReplyMarkup:     groupMenuMarkup(groups),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ select department")
}
func (h *handler) handleTextGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	groupName, err := h.Schedule.ValidateGroupName(ctx, model.GroupName(update.Message.Text))
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось найти группу с таким названием, попробуйте ещё раз",
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось найти группу с таким названием, попробуйте ещё раз",
		})
		return
	}

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	chat.GroupName = &group.GroupName
	chat.DepartmentName = &group.DepartmentName

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
			ReplyMarkup:     botutil.MainMenuMarkup(chat.IsPrivate()),
		})
		return
	}

	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            fmt.Sprintf("Теперь вы в группе %s", group.GroupName),
		ReplyMarkup:     botutil.MainMenuMarkup(chat.IsPrivate()),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled text group")
}

func (h *handler) handleCmdAccess(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := botutil.DeleteMessage(ctx, b, update.Message)
	addHandlerCtxErr(ctx, err)

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if chat.IsPrivate() {
		err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Настройки доступа доступны только в групповых чатах",
		})
		addHandlerCtxErr(ctx, err)
	} else {
		_, err := botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            accessMenuText(chat),
			ReplyMarkup:     accessMenuMarkup(chat.Access),
		})
		addHandlerCtxErr(ctx, err)
	}

	log.Info().Msg("Handled command access")
}
func (h *handler) handleCQSetAccess(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.CallbackQuery.Message.Message
	chatID := message.Chat.ID
	threadID := message.MessageThreadID

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	accessLevel, err := strconv.Atoi(command.Args[0])
	if err != nil {
		addHandlerCtxErr(ctx, err)
		chat.Access = 0
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "Произошла ошибка, установлено значение 0",
		})
		return
	} else {
		chat.Access = model.ChatAccessLevel(accessLevel)
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotUpdateData,
		})
		return
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      update.CallbackQuery.Message.Message.Chat.ID,
		MessageID:   update.CallbackQuery.Message.Message.ID,
		Text:        accessMenuText(chat),
		ReplyMarkup: accessMenuMarkup(chat.Access),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ set access")
}
func accessMenuText(chat *model.Chat) string {
	text := fmt.Sprintf(
		`Текущий уровень доступа: %d

	0 — без ограничений
	1 — настройки только для админов
	2 — все команды только для админов`,
		chat.Access,
	)
	return text
}

func (h *handler) sendDepartmentSelectionMenu(
	ctx context.Context, b *bot.Bot, chat *model.Chat, update *models.Update,
) error {
	messageThreadID := 0
	if m := update.Message; m != nil {
		messageThreadID = m.MessageThreadID
	} else if cq := update.CallbackQuery; cq != nil {
		if m := cq.Message.Message; m != nil {
			messageThreadID = m.MessageThreadID
		}
	}

	if viper.GetBool(config.KeyHandleVacation) {
		t := time.Now()
		if botutil.IsVacation(t) {
			sendVacationAnswer(ctx, b, update, t, true)
			return nil
		}
	}

	departments, err := h.Schedule.GetDepartments(ctx)
	if err != nil {
		return err
	}

	currentGroup := "группа не выбрана"
	if chat.GroupName != nil {
		currentGroup = fmt.Sprintf("Текущая группа: %s", *chat.GroupName)
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateSelectingGroup)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chat.TgChatID,
			MessageThreadID: messageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return err
	}

	_, err = botutil.SendWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Text:            fmt.Sprintf("%s\nВведите название группы или выберите отделение", currentGroup),
		ReplyMarkup:     departmentMenuMarkup(departments),
	})
	return err
}

func departmentMenuMarkup(departments []model.Department) models.InlineKeyboardMarkup {
	keyboard := make([][]models.InlineKeyboardButton, 0)
	for i := 0; i < len(departments); i += 2 {
		row := make([]models.InlineKeyboardButton, 0)
		for j := i; j < len(departments) && j < i+2; j++ {
			row = append(row, models.InlineKeyboardButton{
				Text: departments[j].Name,
				CallbackData: fmt.Sprintf("%s\n%s",
					botutil.CallbackCommandSelectDepartment, departments[j].Name),
			})
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: "Отмена", CallbackData: botutil.CallbackCommandDeleteConfig},
	})
	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}
func groupMenuMarkup(groups []model.Group) models.ReplyKeyboardMarkup {
	keyboard := [][]models.KeyboardButton{{{Text: "Отмена"}}}
	for i := 0; i < len(groups); i += 2 {
		row := make([]models.KeyboardButton, 0)
		for j := i; j < len(groups) && j < i+2; j++ {
			row = append(row, models.KeyboardButton{Text: string(groups[j].GroupName)})
		}
		keyboard = append(keyboard, row)
	}
	return models.ReplyKeyboardMarkup{
		Keyboard:        keyboard,
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
		Selective:       true,
	}

}

func onOffStr(isOn bool) string {
	if isOn {
		return "включено"
	}
	return "выключено"
}

func accessMenuMarkup(accessLevel model.ChatAccessLevel) models.InlineKeyboardMarkup {
	keyboard := [][]models.InlineKeyboardButton{
		{},
		{{Text: "Закрыть", CallbackData: botutil.CallbackCommandDeleteConfig}},
	}
	for i := range 3 {
		text := fmt.Sprint(i)
		if i == int(accessLevel) {
			text = fmt.Sprintf("[%d]", i)
		}
		keyboard[0] = append(keyboard[0], models.InlineKeyboardButton{
			Text:         text,
			CallbackData: fmt.Sprintf("%s\n%d", botutil.CallbackCommandSetAccess, i),
		})
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}
