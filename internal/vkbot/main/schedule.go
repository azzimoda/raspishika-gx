package vkbot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/messenger"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
	"github.com/rs/zerolog/log"
)

var (
	errGroupNotFound   = errors.New("group not found")
	errTeacherNotFound = errors.New("teacher not found")
)

func (b *Bot) groupSchedule(ctx context.Context, chat *model.Chat, msg vkclient.Message, kind, name string, entry *model.UpdateLog) error {
	b.clearSession(msg)
	if strings.TrimSpace(name) == "" {
		if chat.GroupName == nil {
			return b.send(ctx, msg.PeerID, "Группа ещё не выбрана. Откройте /settings или укажите её в команде, например /week испт 22 9 2.", mainKeyboard())
		}
		name = string(*chat.GroupName)
	}
	group, err := b.schedules.GetGroupByName(ctx, model.GroupName(strings.TrimSpace(name)))
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if group == nil {
		return b.send(ctx, msg.PeerID, "Группа не найдена. Выберите её заново через /settings.", nil)
	}
	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	return b.sendSchedule(ctx, msg, conf, kind, model.ScheduleURL(conf, nil), entry)
}

// keyboardSchedule opens a schedule view referenced by the week and day
// buttons. The value is a group name or a numeric teacher ID in the on-screen
// schedule.
func (b *Bot) keyboardSchedule(ctx context.Context, chat *model.Chat, msg vkclient.Message, kind, arg string, entry *model.UpdateLog) error {
	b.clearSession(msg)
	value := strings.TrimSpace(arg)
	if value == "" {
		if chat.GroupName == nil {
			return b.send(ctx, msg.PeerID, "Группа ещё не выбрана. Откройте /settings или выберите её через кнопки.", mainKeyboard())
		}
		value = string(*chat.GroupName)
	}
	if kind == "day" {
		parts := strings.SplitN(value, "\n", 2)
		value = strings.TrimSpace(parts[0])
		if value == "" && chat.GroupName != nil {
			value = string(*chat.GroupName)
		}
		idx := -1
		if len(parts) == 2 {
			idx, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
		return b.dayView(ctx, msg, value, idx, chat.DarkMode, entry)
	}
	return b.weekView(ctx, msg, value, chat.DarkMode, entry)
}

func (b *Bot) weekView(ctx context.Context, msg vkclient.Message, value string, darkMode bool, entry *model.UpdateLog) error {
	conf, link, err := b.resolveScheduleTarget(ctx, value, darkMode)
	if err != nil {
		return b.scheduleTargetFail(ctx, msg, err)
	}
	return b.sendSchedule(ctx, msg, conf, "week", link, entry)
}

func (b *Bot) dayView(ctx context.Context, msg vkclient.Message, value string, idx int, darkMode bool, entry *model.UpdateLog) error {
	conf, link, err := b.resolveScheduleTarget(ctx, value, darkMode)
	if err != nil {
		return b.scheduleTargetFail(ctx, msg, err)
	}
	schedule, err := b.schedules.GetSchedule(ctx, conf)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if schedule == nil || len(schedule.Days) == 0 {
		return b.send(ctx, msg.PeerID, "Расписание пока не опубликовано. Попробуйте позже.", nil)
	}
	if idx < 0 || idx >= len(schedule.Days) {
		return b.send(ctx, msg.PeerID, "На эту дату расписания нет.", nil)
	}
	schedule.Config = conf
	entry.GroupOrTeacher, entry.IsCached = conf.Name(), schedule.IsOld
	markPassed := time.Time{}
	if sameDate(schedule.Days[idx].Date, b.now()) {
		markPassed = b.now()
	}
	note := ""
	if schedule.IsOld {
		note = "\n\nПоказана сохранённая копия расписания: сайт временно не обновил данные."
	}
	return b.send(ctx, msg.PeerID, conf.Name()+"\n\n"+formatDay(schedule.Days[idx], markPassed)+note, vkbotutil.ScheduleKeyboard(navigatorValue(conf), schedule.Days, idx, link))
}

// navigatorValue is the value the on-screen week and day buttons round-trip to
// this bot: a numeric teacher ID for teachers, the group name otherwise.
func navigatorValue(conf model.ScheduleConfig) string {
	if conf.Teacher != nil {
		return conf.Teacher.TeacherID
	}
	return conf.Name()
}

// resolveScheduleTarget builds a schedule config and its page link from a group
// name or numeric teacher ID.
func (b *Bot) resolveScheduleTarget(ctx context.Context, value string, darkMode bool) (model.ScheduleConfig, string, error) {
	if isTeacherID(value) {
		teacher, err := b.schedules.GetTeacherByNameOrID(ctx, value)
		if err != nil {
			return model.ScheduleConfig{}, "", err
		}
		if teacher == nil {
			return model.ScheduleConfig{}, "", errTeacherNotFound
		}
		conf := model.TeacherScheduleConfig(teacher, darkMode)
		departments, err := b.schedules.GetDepartments(ctx)
		if err != nil {
			return model.ScheduleConfig{}, "", err
		}
		return conf, model.ScheduleURL(conf, departments), nil
	}
	group, err := b.schedules.GetGroupByName(ctx, model.GroupName(value))
	if err != nil {
		return model.ScheduleConfig{}, "", err
	}
	if group == nil {
		return model.ScheduleConfig{}, "", errGroupNotFound
	}
	conf := model.GroupScheduleConfig(group, darkMode)
	return conf, model.ScheduleURL(conf, nil), nil
}

func (b *Bot) scheduleTargetFail(ctx context.Context, msg vkclient.Message, err error) error {
	if errors.Is(err, errGroupNotFound) {
		return b.send(ctx, msg.PeerID, "Группа не найдена. Выберите её заново через /settings.", nil)
	}
	if errors.Is(err, errTeacherNotFound) {
		return b.send(ctx, msg.PeerID, "Преподаватель не найден. Отправьте /teacher для нового поиска.", nil)
	}
	return b.fail(ctx, msg, err)
}

// sendSchedule sends the week photo or the today/tomorrow day text, depending
// on kind, and records the update statistics entry.
func (b *Bot) sendSchedule(ctx context.Context, msg vkclient.Message, conf model.ScheduleConfig, kind, link string, entry *model.UpdateLog) error {
	schedule, err := b.schedules.GetSchedule(ctx, conf)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if schedule == nil || len(schedule.Days) == 0 {
		return b.send(ctx, msg.PeerID, "Расписание пока не опубликовано. Попробуйте позже.", nil)
	}
	schedule.Config = conf
	entry.GroupOrTeacher, entry.IsCached = conf.Name(), schedule.IsOld
	keyboard := vkbotutil.ScheduleKeyboard(navigatorValue(conf), schedule.Days, -1, link)
	note := ""
	if schedule.IsOld {
		note = "\n\nПоказана сохранённая копия расписания: сайт временно не обновил данные."
	}
	if kind == "week" {
		filename, data, imageErr := b.schedules.PrepareScheduleImage(ctx, schedule)
		if imageErr == nil && len(data) > 0 {
			imageErr = b.sendPhoto(ctx, msg.PeerID, filepath.Base(filename), data, "Расписание — "+conf.Name()+note, keyboard)
			if imageErr == nil {
				return nil
			}
			if vkclient.IsForbidden(imageErr) || ctx.Err() != nil {
				return imageErr
			}
		}
		// Text is a useful fallback if Chromium is temporarily unavailable.
		log.Warn().Err(imageErr).Msg("VK schedule image unavailable; sending text")
		var text strings.Builder
		fmt.Fprintf(&text, "Расписание — %s\n", conf.Name())
		for _, day := range schedule.Days {
			text.WriteString("\n" + formatDay(day, time.Time{}) + "\n")
		}
		text.WriteString(note)
		return b.send(ctx, msg.PeerID, text.String(), keyboard)
	}
	now := b.now()
	wanted := now
	if kind == "tomorrow" {
		wanted = wanted.AddDate(0, 0, 1)
	}
	day, ok := scheduleDay(schedule.Days, wanted, kind, now)
	if !ok {
		text := fmt.Sprintf("%s · %s\nНа эту дату расписания нет.", conf.Name(), wanted.Format("02.01.2006"))
		if wanted.Weekday() == time.Sunday {
			text = fmt.Sprintf("%s · %s\nВоскресенье — пар нет.", conf.Name(), wanted.Format("02.01.2006"))
		}
		return b.send(ctx, msg.PeerID, text+note, keyboard)
	}
	markPassed := time.Time{}
	if kind == "today" {
		markPassed = now
	}
	return b.send(ctx, msg.PeerID, conf.Name()+"\n\n"+formatDay(day, markPassed)+note, vkbotutil.ScheduleKeyboard(navigatorValue(conf), schedule.Days, dayIndex(schedule.Days, day), link))
}

func (b *Bot) sendPhoto(ctx context.Context, peerID int64, filename string, data []byte, caption string, keyboard *vkbotutil.Keyboard) error {
	_, err := b.api.SendPhoto(ctx, peerID, filename, data, caption, keyboard)
	return err
}

// Prefer dates over array positions: cached schedules may start on another day,
// and Sundays may be omitted altogether by the source site.
func scheduleDay(days []model.ScheduleDay, wanted time.Time, kind string, now time.Time) (model.ScheduleDay, bool) {
	parsedAny := false
	for _, day := range days {
		for _, layout := range []string{"02.01.2006", "2006-01-02", "02.01.06", "2.1.2006"} {
			parsed, err := time.ParseInLocation(layout, strings.TrimSpace(day.Date), wanted.Location())
			if err != nil {
				continue
			}
			parsedAny = true
			if parsed.Year() == wanted.Year() && parsed.YearDay() == wanted.YearDay() {
				return day, true
			}
			break
		}
	}
	if parsedAny || wanted.Weekday() == time.Sunday {
		return model.ScheduleDay{}, false
	}
	index := 0
	if kind == "tomorrow" && now.Weekday() != time.Sunday {
		index = 1
	}
	if index < len(days) {
		return days[index], true
	}
	return model.ScheduleDay{}, false
}

func sameDate(dayDate string, now time.Time) bool {
	day, err := parseDayDate(dayDate, now.Location())
	if err != nil {
		return false
	}
	return day.Year() == now.Year() && day.YearDay() == now.YearDay()
}

func parseDayDate(value string, loc *time.Location) (time.Time, error) {
	for _, layout := range []string{"02.01.2006", "2006-01-02", "02.01.06", "2.1.2006"} {
		if parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("unknown schedule date layout")
}

func dayIndex(days []model.ScheduleDay, wanted model.ScheduleDay) int {
	for i, day := range days {
		if day.Date == wanted.Date {
			return i
		}
	}
	return -1
}

func formatDay(day model.ScheduleDay, current time.Time) string {
	var out strings.Builder
	fmt.Fprintf(&out, "📅 %s, %s", day.Weekday, day.Date)
	if day.WeekKind != "" {
		fmt.Fprintf(&out, " · %s", day.WeekKind)
	}
	count := 0
	for _, pair := range day.Pairs {
		if pair.Kind == model.PairKindEmpty && !pair.Replaced {
			continue
		}
		count++
		out.WriteString("\n\n")
		if !current.IsZero() && pair.IsPassedAt(current) {
			out.WriteString("✓ Завершена · ")
		}
		fmt.Fprintf(&out, "%d | %s–%s", pair.Number, pair.StartTime, pair.EndTime)
		if pair.Classroom != "" {
			fmt.Fprintf(&out, " | %s", pair.Classroom)
		}
		if pair.Replaced {
			out.WriteString(" · Изменение")
		}
		if pair.Kind == model.PairKindEmpty {
			out.WriteString("\nСнято")
			continue
		}
		if pair.Title != "" {
			out.WriteString("\n" + messenger.PlainText(pair.Title))
		}
		if pair.Discipline != "" {
			out.WriteString("\n" + messenger.PlainText(pair.Discipline))
		} else if pair.Label != "" {
			out.WriteString("\n" + messenger.PlainText(pair.Label))
		}
		if pair.Subgroup != "" {
			out.WriteString("\nПодгруппа: " + messenger.PlainText(pair.Subgroup))
		}
		if pair.Teacher != "" {
			out.WriteString("\n" + messenger.PlainText(pair.Teacher))
		}
		if pair.Group != "" {
			out.WriteString("\nГруппа: " + messenger.PlainText(pair.Group))
		}
	}
	if count == 0 {
		out.WriteString("\nПар нет.")
	}
	return out.String()
}

func (b *Bot) teacherSearch(ctx context.Context, chat *model.Chat, msg vkclient.Message, query string, entry *model.UpdateLog) error {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 {
		return b.send(ctx, msg.PeerID, "Введите фамилию или часть имени преподавателя (до 100 символов).", nil)
	}
	b.setSession(msg, "teacher", query)
	if query == "" {
		return b.teacherPage(ctx, chat, msg, "0")
	}
	teachers, err := b.schedules.FindTeachersByName(ctx, query)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if len(teachers) == 0 {
		return b.send(ctx, msg.PeerID, "Преподаватель не найден. Попробуйте другую фамилию или её часть. Отмена — /cancel.", nil)
	}
	if len(teachers) == 1 {
		return b.teacherSchedule(ctx, chat, msg, teachers[0].TeacherID, entry)
	}
	return b.sendTeacherChoices(ctx, msg, teachers, 0, false)
}

func (b *Bot) teacherPage(ctx context.Context, chat *model.Chat, msg vkclient.Message, arg string) error {
	state := b.getSession(msg)
	if state.kind != "teacher" {
		return b.send(ctx, msg.PeerID, "Поиск истёк. Отправьте /teacher и фамилию преподавателя.", nil)
	}
	if state.query != "" {
		teachers, err := b.schedules.FindTeachersByName(ctx, state.query)
		if err != nil {
			return b.fail(ctx, msg, err)
		}
		return b.sendTeacherChoices(ctx, msg, teachers, pageNumber(arg), false)
	}
	recent, err := b.chats.GetRecentTeachers(ctx, chat.ID)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	teachers := make([]model.Teacher, 0, len(recent))
	for _, teacher := range recent {
		if teacher != nil {
			teachers = append(teachers, model.Teacher{TeacherID: teacher.TeacherID, Name: teacher.TeacherName})
		}
	}
	return b.sendTeacherChoices(ctx, msg, teachers, pageNumber(arg), true)
}

func (b *Bot) sendTeacherChoices(ctx context.Context, msg vkclient.Message, teachers []model.Teacher, page int, recent bool) error {
	text := "Найдены преподаватели. Выберите нужного или уточните фамилию сообщением."
	if recent {
		text = "Отправьте фамилию или часть имени преподавателя. Можно выбрать из недавних запросов ниже. Отмена — /cancel."
	}
	if len(teachers) == 0 {
		return b.send(ctx, msg.PeerID, "Отправьте фамилию или часть имени преподавателя. Отмена — /cancel.", &vkbotutil.Keyboard{Inline: true, Buttons: [][]vkbotutil.Button{{vkbotutil.TextButton("Отмена", "cancel")}}})
	}
	items := make([]vkbotutil.MenuItem, 0, len(teachers))
	for _, teacher := range teachers {
		items = append(items, vkbotutil.MenuItem{Label: teacher.Name, Command: "showteacher\n" + teacher.TeacherID})
	}
	return b.send(ctx, msg.PeerID, text, vkbotutil.PagedKeyboard(items, page, "teachers", "cancel"))
}

func (b *Bot) teacherSchedule(ctx context.Context, chat *model.Chat, msg vkclient.Message, id string, entry *model.UpdateLog) error {
	if strings.TrimSpace(id) == "" {
		return b.teacherSearch(ctx, chat, msg, "", entry)
	}
	teacher, err := b.schedules.GetTeacherByNameOrID(ctx, id)
	if err != nil {
		return b.fail(ctx, msg, err)
	}
	if teacher == nil {
		return b.send(ctx, msg.PeerID, "Преподаватель не найден. Отправьте /teacher для нового поиска.", nil)
	}
	conf := model.TeacherScheduleConfig(teacher, chat.DarkMode)
	departments, err := b.schedules.GetDepartments(ctx)
	link := ""
	if err == nil {
		link = model.ScheduleURL(conf, departments)
	}
	if err := b.sendSchedule(ctx, msg, conf, "week", link, entry); err != nil {
		return err
	}
	b.clearSession(msg)
	if err := b.chats.AddChatRecentTeacher(ctx, &model.RecentTeacher{ChatID: chat.ID, TeacherID: teacher.TeacherID, TeacherName: teacher.Name}); err != nil {
		log.Warn().Err(err).Msg("Failed to save VK recent teacher")
	}
	return nil
}

// isTeacherID reports whether the value is a numeric teacher ID rather than a
// group name.
func isTeacherID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
