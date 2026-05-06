package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type ChatRepository interface {
	Create(context.Context, *model.Chat) error
	CreateOrUpdate(context.Context, *model.Chat) (created bool, err error)

	Update(context.Context, *model.Chat) error

	Get(ctx context.Context, id int64) (*model.Chat, error)
	GetByChatID(context.Context, model.ChatID) (*model.Chat, error)

	GetAll(context.Context) ([]*model.Chat, error)
	GetAllPrivate(context.Context) ([]*model.Chat, error)
	GetAllNew(context.Context, time.Duration) ([]*model.Chat, error)
	GetAllByGroup(context.Context, model.GroupName) ([]*model.Chat, error)
	GetAllByWatchedGroup(context.Context, model.GroupName) ([]*model.Chat, error)
	GetAllByDailyTime(ctx context.Context, time string) ([]*model.Chat, error)
	GetAllWithPairNotification(context.Context) ([]*model.Chat, error)
	GetAllWithChangeAlert(context.Context) ([]*model.Chat, error)
	GetAllWithDarkMode(context.Context) ([]*model.Chat, error)

	CountAll(context.Context) (int, error)
	CountAllPrivate(context.Context) (int, error)
	CountAllNew(context.Context, time.Duration) (int, error)
	CountInactive(context.Context, time.Duration) (int, error)
	GetAvgChatPerGroup(context.Context) (float64, error)

	Delete(ctx context.Context, id int64) error

	CountAllConfiguredGroups(context.Context) (int, error)
	GetWatchedGroups(context.Context) ([]*model.Group, error)

	AddRecentTeacher(ctx context.Context, chatID, teacherID int64) error
	RecentTeachers(ctx context.Context, chatID int64) ([]*model.RecentTeacher, error)
}

func NewChatRepository(db *sqlx.DB) ChatRepository { return &chatRepository{db: db} }

type chatRepository struct{ db *sqlx.DB }

func (r *chatRepository) Create(ctx context.Context, chat *model.Chat) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO chats (tg_chat_id, username) VALUES (?,?)`, chat.TgChatID, chat.UserName)
	if err != nil {
		return err
	}
	chat.ID, err = res.LastInsertId()
	return nil
}
func (r *chatRepository) CreateOrUpdate(ctx context.Context, chat *model.Chat) (created bool, err error) {
	var existingChat model.Chat
	err = r.db.GetContext(ctx, &existingChat, `SELECT * FROM chats WHERE tg_chat_id = ?`, chat.TgChatID)

	if err == sql.ErrNoRows {
		// Create new chat
		log.Debug().Any("tgChatID", chat.TgChatID).Msg("Chat does not exist, creating...")
		if err := r.Create(ctx, chat); err != nil {
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
		if err := r.Update(ctx, &existingChat); err != nil {
			return false, fmt.Errorf("failed to update chat's username (%v -> %s): %w",
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

func (r *chatRepository) Update(ctx context.Context, chat *model.Chat) error {
	_, err := r.db.NamedExecContext(ctx, `
			UPDATE chats
			SET username = :username,
				state = :state,
				department = :department,
				"group" = :group,
				daily_sending_time = :daily_sending_time,
				pair_sending = :pair_sending,
				update_notification = :update_notification,
				access = :access,
				dark_mode = :dark_mode,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = :id
		`, chat)
	return err
}

func (r *chatRepository) Get(ctx context.Context, id int64) (*model.Chat, error) {
	var chat model.Chat
	err := r.db.GetContext(ctx, &chat, `SELECT * FROM chats WHERE id = ?`, id)
	return &chat, err
}
func (r *chatRepository) GetByChatID(ctx context.Context, chatID model.ChatID) (*model.Chat, error) {
	chat := model.Chat{}
	err := r.db.GetContext(ctx, &chat, `SELECT * FROM chats WHERE tg_chat_id = ?`, chatID)
	return &chat, err
}

func (r *chatRepository) GetAll(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats`)
	return chats, err
}
func (r *chatRepository) GetAllPrivate(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE tg_chat_id > 0`)
	return chats, err
}
func (r *chatRepository) GetAllNew(ctx context.Context, dur time.Duration) ([]*model.Chat, error) {
	var chats []*model.Chat
	if err := r.db.SelectContext(
		ctx,
		&chats,
		`SELECT * FROM chats WHERE created_at > datetime('now', ?)`,
		sqlPeriod(dur),
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return chats, nil
}
func (r *chatRepository) GetAllByGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE "group" = ?`, group)
	return chats, err
}
func (r *chatRepository) GetAllByWatchedGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE "group" = ? AND update_notification = 1`, group)
	return chats, err
}
func (r *chatRepository) GetAllByDailyTime(ctx context.Context, time string) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE "group" IS NOT NULL AND daily_sending_time = ?`, time)
	return chats, err
}
func (r *chatRepository) GetAllWithPairNotification(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE "group" IS NOT NULL AND "group" != '' AND pair_sending = 1`)
	return chats, err
}
func (r *chatRepository) GetAllWithChangeAlert(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE update_notification = 1`)
	return chats, err
}
func (r *chatRepository) GetAllWithDarkMode(ctx context.Context) ([]*model.Chat, error) {
	var chats []*model.Chat
	err := r.db.SelectContext(ctx, &chats, `SELECT * FROM chats WHERE dark_mode = 1`)
	return chats, err
}

func (r *chatRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `SELECT count(*) FROM chats`)
	return count, err
}
func (r *chatRepository) CountAllPrivate(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `SELECT count(*) FROM chats WHERE tg_chat_id > 0`)
	return count, err
}
func (r *chatRepository) CountAllNew(ctx context.Context, dur time.Duration) (int, error) {
	var count int
	period := sqlPeriod(dur)
	err := r.db.GetContext(ctx, &count, `
		SELECT count(*) FROM chats
		WHERE updated_at > datetime('now', ?)
	`, period)
	return count, err
}
func (r *chatRepository) CountInactive(ctx context.Context, dur time.Duration) (int, error) {
	var count int
	period := sqlPeriod(dur)
	err := r.db.GetContext(ctx, &count, `
			SELECT count(*) FROM (
				SELECT c.id, c.tg_chat_id, c.username, c."group", c.daily_sending_time, c.pair_sending, c.updated_at,
					count(ul.id) as count FROM chats c
				LEFT JOIN (
					SELECT * FROM update_logs WHERE created_at > datetime('now', ?)
				) ul ON c.id = ul.chat_id
				GROUP BY c.id
				HAVING count = 0 AND ("group" IS NULL OR "group" = '' OR daily_sending_time IS NULL AND pair_sending = 0)
			);
		`, period, period)
	return count, err
}
func (r *chatRepository) GetAvgChatPerGroup(ctx context.Context) (float64, error) {
	var avg float64
	err := r.db.GetContext(ctx, &avg, `
		SELECT AVG(chats) FROM (
			SELECT COUNT(*) AS chats FROM chats
			WHERE "group" != '' AND "group" IS NOT NULL
			GROUP BY "group"
		)
	`)
	return avg, err
}

func (r *chatRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM chats WHERE id = ?`, id)
	return err
}

func (r *chatRepository) CountAllConfiguredGroups(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT count(*) from (
			SELECT DISTINCT g.* FROM groups g JOIN chats c ON g.group_name = c."group"
			WHERE "group" != '' AND "group" IS NOT NULL
		)
	`)
	return count, err
}
func (r *chatRepository) GetWatchedGroups(ctx context.Context) ([]*model.Group, error) {
	var groups []*model.Group
	err := r.db.SelectContext(ctx, &groups, `
		SELECT DISTINCT g.* FROM groups g JOIN chats c ON g.group_name = c."group"
		WHERE "group" != '' AND "group" IS NOT NULL AND update_notification = 1
	`)
	return groups, err
}

// TODO: Refactor
func (r *chatRepository) AddRecentTeacher(ctx context.Context, chatID, teacherID int64) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM recent_teachers WHERE chat_id = ? AND teacher_id = ?`,
		chatID,
		teacherID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete same recent teachers (%d) of chat (%d): %w", teacherID, chatID, err)
	}

	rt, err := r.RecentTeachers(ctx, chatID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get recent teachers (%d) of chat (%d): %w", teacherID, chatID, err)
	}

	if len(rt) >= 4 {
		if _, err := tx.NamedExecContext(
			ctx,
			`DELETE FROM recent_teachers WHERE chat_id = :chat_id AND teacher_id = :teacher_id`,
			rt[0],
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete oldest recent teacher (%d) of chat (%d): %w",
				rt[0].TeacherID, chatID, err)
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO recent_teachers (chat_id, teacher_id) VALUES (?,?)`,
		chatID,
		teacherID,
	); err != nil {
		return fmt.Errorf("failed to insert recent teacher: %w", err)
	}

	return tx.Commit()
}
func (r *chatRepository) RecentTeachers(ctx context.Context, chatID int64) ([]*model.RecentTeacher, error) {
	var rt []*model.RecentTeacher
	err := r.db.SelectContext(ctx, &rt, `SELECT * FROM recent_teachers WHERE chat_id = ? ORDER BY created_at ASC`, chatID)
	return rt, err
}
