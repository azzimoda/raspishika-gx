package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type ChatRepository interface {
	CreateChat(context.Context, *model.Chat) error
	CreateOrUpdateChat(context.Context, *model.Chat) (created bool, err error)

	UpdateChat(context.Context, *model.Chat) error

	GetChat(ctx context.Context, id int64) (*model.Chat, error)
	GetChatByChatID(context.Context, model.ChatID) (*model.Chat, error)
	GetChatByUsernameOrChatID(context.Context, string) (*model.Chat, error)

	GetAllChats(context.Context) ([]*model.Chat, error)
	GetPrivateChats(context.Context) ([]*model.Chat, error)
	GetNewChats(context.Context, time.Duration) ([]*model.Chat, error)
	GetChatsByGroup(context.Context, model.GroupName) ([]*model.Chat, error)
	GetChatsByWatchedGroup(context.Context, model.GroupName) ([]*model.Chat, error)
	GetChatsWithDailyTime(ctx context.Context, time string) ([]*model.Chat, error)
	GetChatsWithPairNotification(context.Context) ([]*model.Chat, error)
	GetChatsWithChangeAlert(context.Context) ([]*model.Chat, error)
	GetChatsWithDarkMode(context.Context) ([]*model.Chat, error)

	CountAllChats(context.Context) (int, error)
	CountPricateChats(context.Context) (int, error)
	CountNewChats(context.Context, time.Duration) (int, error)
	GetNewChatCountByYear(context.Context, time.Duration) (map[int]int, error)
	// CountChatActivities returns the number of chats split by activity level
	// within the given period.
	CountChatActivities(context.Context, time.Duration) (ChatActivityCounts, error)
	CountChatsWithConfiguredGroup(context.Context) (int, error)
	CountUniqueConfiguredGroups(context.Context) (int, error)
	CountDailyEnabled(context.Context) (int, error)
	CountPairEnabled(context.Context) (int, error)
	CountChangeEnabled(context.Context) (int, error)
	CountDarkEnabled(context.Context) (int, error)
	GetAvgChatPerGroup(context.Context) (float64, error)
	GetGroupedCountChatCountByTime(context.Context) ([]TimeCount, error)
	// GetChatCountByDepartment returns the number of chats per department,
	// ordered by count descending.
	GetChatCountByDepartment(context.Context) ([]NameCount, error)
	// GetChatsByAccessLevel returns the number of chats per access level.
	GetChatsByAccessLevel(context.Context) (map[model.ChatAccessLevel]int, error)
	// GetTopGroupsByChatCount returns the configured groups with the most chats.
	GetTopGroupsByChatCount(ctx context.Context, limit int) ([]NameCount, error)
	// CountPrivateChatsWithConfiguredGroup returns the number of private chats
	// (tg_chat_id > 0) that have a group configured.
	CountPrivateChatsWithConfiguredGroup(context.Context) (int, error)

	DeleteChat(ctx context.Context, id int64) error

	CountAllConfiguredGroups(context.Context) (int, error)
	GetWatchedGroupNames(context.Context) ([]string, error)

	AddRecentTeacher(context.Context, *model.RecentTeacher) error
	GetRecentTeachers(ctx context.Context, chatID int64) ([]*model.RecentTeacher, error)
}

func NewChatRepository(db *gorm.DB) ChatRepository { return &chatRepository{db: db} }

type chatRepository struct{ db *gorm.DB }

func (r *chatRepository) CreateChat(ctx context.Context, chat *model.Chat) error {
	return r.db.WithContext(ctx).Create(chat).Error
}
func (r *chatRepository) CreateOrUpdateChat(ctx context.Context, chat *model.Chat) (created bool, err error) {
	var existingChat model.Chat
	err = r.db.WithContext(ctx).Where("tg_chat_id = ?", chat.TgChatID).First(&existingChat).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new chat
		log.Debug().Any("tgChatID", chat.TgChatID).Msg("Chat does not exist, creating...")
		if err := r.CreateChat(ctx, chat); err != nil {
			return true, fmt.Errorf("failed to create chat (%v): %w", chat, err)
		}
		return true, nil
	}
	if err != nil {
		log.Error().Err(err).Any("tgChatID", chat.TgChatID).Msg("Failed to get chat")
		return false, fmt.Errorf("failed to get chat by Telegram chat ID (%d): %w", chat.TgChatID, err)
	}

	if refutil.DerefOrTypeDefault(existingChat.UserName) != *chat.UserName {
		// Update username
		existingChat.UserName = chat.UserName
		if err := r.UpdateChat(ctx, &existingChat); err != nil {
			return false, fmt.Errorf("failed to update chat's username (%v -> %v): %w",
				existingChat.UserName, chat.UserName, err)
		}
		*chat = existingChat
		return false, nil
	}

	// Return existing chat
	*chat = existingChat
	log.Trace().Any("tgChatID", existingChat.TgChatID).Msg("Chat already exists")
	return false, nil
}

func (r *chatRepository) UpdateChat(ctx context.Context, chat *model.Chat) error {
	return r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("id = ?", chat.ID).
		Updates(map[string]any{
			"username":            chat.UserName,
			"state":               chat.State,
			"department":          chat.DepartmentName,
			"group":               chat.GroupName,
			"daily_sending_time":  chat.DailySendingTime,
			"pair_sending":        chat.PairSending,
			"update_notification": chat.ChangeAlert,
			"access":              chat.Access,
			"dark_mode":           chat.DarkMode,
			"updated_at":          time.Now(),
		}).Error
}

func (r *chatRepository) GetChat(ctx context.Context, id int64) (*model.Chat, error) {
	var chat model.Chat
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&chat).Error
	return &chat, err
}
func (r *chatRepository) GetChatByChatID(ctx context.Context, chatID model.ChatID) (*model.Chat, error) {
	chat := model.Chat{}
	err := r.db.WithContext(ctx).Where("tg_chat_id = ?", chatID).First(&chat).Error
	return &chat, err
}
func (r *chatRepository) GetChatByUsernameOrChatID(ctx context.Context, usernameOrChatID string) (*model.Chat, error) {
	chat := model.Chat{}
	username := strings.TrimPrefix(usernameOrChatID, "@")
	var err error
	if chatID, parseErr := strconv.ParseInt(username, 10, 64); parseErr == nil {
		err = r.db.WithContext(ctx).Where("tg_chat_id = ?", chatID).First(&chat).Error
	} else {
		err = r.db.WithContext(ctx).Where("username = ?", username).First(&chat).Error
	}
	return &chat, err
}

func (r *chatRepository) GetAllChats(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetPrivateChats(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).Where("tg_chat_id > 0").Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetNewChats(ctx context.Context, dur time.Duration) ([]*model.Chat, error) {
	var chats []*model.Chat
	if err := r.db.WithContext(ctx).
		Where("created_at > datetime('now', ?)", sqlPeriod(dur)).
		Find(&chats).Error; err != nil {
		return nil, err
	}
	return chats, nil
}
func (r *chatRepository) GetChatsByGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).Where("group = ?", group).Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetChatsByWatchedGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).
		Where("group = ? AND update_notification = 1", group).
		Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetChatsWithDailyTime(ctx context.Context, time string) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).
		Where(`"group" IS NOT NULL AND daily_sending_time = ?`, time).
		Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetChatsWithPairNotification(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).
		Where(`"group" IS NOT NULL AND "group" != '' AND pair_sending = 1`).
		Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetChatsWithChangeAlert(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).Where("update_notification = 1").Find(&chats).Error
	return chats, err
}
func (r *chatRepository) GetChatsWithDarkMode(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.WithContext(ctx).Where("dark_mode = 1").Find(&chats).Error
	return chats, err
}

func (r *chatRepository) CountAllChats(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Chat{}).Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountPricateChats(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Chat{}).Where("tg_chat_id > 0").Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountNewChats(ctx context.Context, dur time.Duration) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("created_at > datetime('now', ?)", sqlPeriod(dur)).
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) GetNewChatCountByYear(ctx context.Context, dur time.Duration) (map[int]int, error) {
	result := make([]struct {
		Group *string
		Count int
	}, 0)
	period := sqlPeriod(dur)
	err := r.db.WithContext(ctx).Raw(`
		SELECT "group", count(*) AS count
		FROM chats
		WHERE created_at > datetime('now', ?)
		GROUP BY "group"
	`, period).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	m := make(map[int]int)
	for _, r := range result {
		groupName := model.GroupName(refutil.DerefOrTypeDefault(r.Group))
		year := 0
		if groupName != "" {
			if _, year, _, _, err = groupName.Parse(); err != nil {
				continue
			}
		}
		m[year] += r.Count
	}
	return m, nil
}

// ChatActivityCounts holds the number of chats split by their activity level
// within a given period.
type ChatActivityCounts struct {
	Active     int
	Semiactive int
	Inactive   int
}

// CountChatActivities counts active, semiactive and inactive chats in a single
// pass. Each chat falls into exactly one bucket:
//   - active: at least one update log within the period;
//   - semiactive: no logs within the period, but a group configured and at
//     least one of daily/pair/change broadcast enabled;
//   - inactive: no logs within the period and otherwise (no group or no
//     broadcast enabled).
func (r *chatRepository) CountChatActivities(ctx context.Context, dur time.Duration) (ChatActivityCounts, error) {
	const query = `
		SELECT
			COALESCE(SUM(CASE WHEN cnt > 0 THEN 1 ELSE 0 END), 0) AS active,
			COALESCE(SUM(CASE WHEN cnt = 0 AND has_group AND has_broadcast THEN 1 ELSE 0 END), 0) AS semiactive,
			COALESCE(SUM(CASE WHEN cnt = 0 AND (NOT has_group OR NOT has_broadcast) THEN 1 ELSE 0 END), 0) AS inactive
		FROM (
			SELECT c.id,
				COUNT(ul.id) AS cnt,
				("group" IS NOT NULL AND "group" != '') AS has_group,
				(daily_sending_time IS NOT NULL OR pair_sending = 1 OR update_notification = 1) AS has_broadcast
			FROM chats c
			LEFT JOIN update_logs ul
				ON ul.chat_id = c.id AND ul.created_at > datetime('now', ?)
			GROUP BY c.id
		)
	`
	var counts ChatActivityCounts
	err := r.db.WithContext(ctx).Raw(query, sqlPeriod(dur)).Scan(&counts).Error
	return counts, err
}

func (r *chatRepository) CountChatsWithConfiguredGroup(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where(`"group" IS NOT NULL AND "group" != ''`).
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountUniqueConfiguredGroups(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where(`"group" IS NOT NULL AND "group" != ''`).
		Distinct("group").
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountDailyEnabled(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("daily_sending_time IS NOT NULL AND daily_sending_time != ''").
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountPairEnabled(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("pair_sending = 1").
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountChangeEnabled(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("update_notification = 1").
		Count(&count).Error
	return int(count), err
}
func (r *chatRepository) CountDarkEnabled(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("dark_mode = 1").
		Count(&count).Error
	return int(count), err
}

// GetAvgChatPerGroup returns the average number of chats per group.
func (r *chatRepository) GetAvgChatPerGroup(ctx context.Context) (float64, error) {
	const query = `
		SELECT AVG(chats) FROM (
			SELECT COUNT(*) AS chats FROM chats
			WHERE "group" IS NOT NULL AND "group" != ''
			GROUP BY "group"
		)
	`
	var avg *float64
	err := r.db.WithContext(ctx).Raw(query).Scan(&avg).Error
	return refutil.DerefOrTypeDefault(avg), err
}

func (r *chatRepository) GetGroupedCountChatCountByTime(ctx context.Context) ([]TimeCount, error) {
	const query = `
		SELECT daily_sending_time AS time, count(*) AS count FROM chats
		WHERE "group" IS NOT NULL AND "group" != ''
			AND daily_sending_time IS NOT NULL AND daily_sending_time != ''
		GROUP BY daily_sending_time
		ORDER BY daily_sending_time
	`
	var result []TimeCount
	err := r.db.WithContext(ctx).Raw(query).Scan(&result).Error
	return result, err
}

func (r *chatRepository) GetChatCountByDepartment(ctx context.Context) ([]NameCount, error) {
	const query = `
		SELECT COALESCE(NULLIF(department, ''), 'unknown') AS name, count(*) AS count
		FROM chats
		GROUP BY COALESCE(NULLIF(department, ''), 'unknown')
	`
	var result []NameCount
	err := r.db.WithContext(ctx).Raw(query).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return mergeNameCounts(result), nil
}

// mergeNameCounts объединяет строки по нормализованному имени отделения
// и сортирует по убыванию количества, затем по имени.
func mergeNameCounts(rows []NameCount) []NameCount {
	merged := make(map[string]int, len(rows))
	for _, row := range rows {
		merged[normalizeDepartmentName(row.Name)] += row.Count
	}
	out := make([]NameCount, 0, len(merged))
	for name, count := range merged {
		out = append(out, NameCount{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// normalizeDepartmentName приводит устаревшие названия отделений
// (префикс «Отделение …» из старого Playwright-скрейпера) к каноническому виду.
func normalizeDepartmentName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" || s == "unknown" {
		return "unknown"
	}
	s = strings.TrimPrefix(s, "Отделение ")
	if s == "ПО" {
		return "Политехническое"
	}
	return s
}

func (r *chatRepository) GetChatsByAccessLevel(ctx context.Context) (map[model.ChatAccessLevel]int, error) {
	const query = `
		SELECT access, count(*) AS count FROM chats
		GROUP BY access
		ORDER BY access
	`
	var result []struct {
		Access model.ChatAccessLevel
		Count  int
	}
	err := r.db.WithContext(ctx).Raw(query).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	m := make(map[model.ChatAccessLevel]int, len(result))
	for _, row := range result {
		m[row.Access] = row.Count
	}
	return m, nil
}

func (r *chatRepository) GetTopGroupsByChatCount(ctx context.Context, limit int) ([]NameCount, error) {
	const query = `
		SELECT "group" AS name, count(*) AS count FROM chats
		WHERE "group" IS NOT NULL AND "group" != ''
		GROUP BY "group"
		ORDER BY count DESC, name ASC
		LIMIT ?
	`
	var result []NameCount
	err := r.db.WithContext(ctx).Raw(query, limit).Scan(&result).Error
	return result, err
}

func (r *chatRepository) CountPrivateChatsWithConfiguredGroup(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Chat{}).
		Where("tg_chat_id > 0 AND \"group\" IS NOT NULL AND \"group\" != ''").
		Count(&count).Error
	return int(count), err
}

type TimeCount struct {
	Time  string
	Count int
}

func (r *chatRepository) DeleteChat(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Chat{}, id).Error
}

func (r *chatRepository) CountAllConfiguredGroups(ctx context.Context) (int, error) {
	const query = `
		SELECT count(*) FROM (
			SELECT DISTINCT "group" FROM chats
			WHERE "group" IS NOT NULL AND "group" != ''
		)
	`
	var count int
	err := r.db.WithContext(ctx).Raw(query).Scan(&count).Error
	return count, err
}

// TODO: Use in config stats
func (r *chatRepository) GetWatchedGroupNames(ctx context.Context) ([]string, error) {
	const query = `
		SELECT DISTINCT "group" FROM chats
		WHERE "group" IS NOT NULL AND "group" != '' AND update_notification = 1
	`
	var groupNames []string
	err := r.db.WithContext(ctx).Raw(query).Scan(&groupNames).Error
	return groupNames, err
}

func (r *chatRepository) AddRecentTeacher(ctx context.Context, recentTeacher *model.RecentTeacher) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing row with same teacher
		if err := tx.
			Where("chat_id = ? AND teacher_id = ?", recentTeacher.ChatID, recentTeacher.TeacherID).
			Delete(&model.RecentTeacher{}).Error; err != nil {
			return fmt.Errorf("failed to delete same recent teacher: %w", err)
		}

		// Get recent teachers.
		var rt []*model.RecentTeacher
		if err := tx.Where("chat_id = ?", recentTeacher.ChatID).
			Order("created_at ASC").
			Find(&rt).Error; err != nil {
			return fmt.Errorf("failed to get recent teachers: %w", err)
		}

		if len(rt) >= 4 {
			// Delete oldest recent teacher.
			if err := tx.
				Where("chat_id = ? AND teacher_id = ?", rt[0].ChatID, rt[0].TeacherID).
				Delete(&model.RecentTeacher{}).Error; err != nil {
				return fmt.Errorf("failed to delete oldest recent teacher: %w", err)
			}
		}

		// Add new recent teacher.
		if err := tx.Create(recentTeacher).Error; err != nil {
			return fmt.Errorf("failed to insert recent teacher: %w", err)
		}

		return nil
	})
}
func (r *chatRepository) GetRecentTeachers(ctx context.Context, chatID int64) ([]*model.RecentTeacher, error) {
	var rt []*model.RecentTeacher
	err := r.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("created_at ASC").
		Find(&rt).Error
	return rt, err
}
