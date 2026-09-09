package repository

import (
	"context"
	"errors"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPrimarySubscription = errors.New("основную группу можно заменить через настройки")

type ScheduleSubscriptionRepository interface {
	GetScheduleSubscriptions(context.Context, int64) ([]model.ScheduleSubscription, error)
	AddScheduleSubscription(context.Context, int64, model.Group) (bool, error)
	SetPrimaryGroup(context.Context, int64, model.Group) (*model.Chat, error)
	RemoveScheduleSubscription(context.Context, int64, int64) (bool, error)
	RemoveUnavailableGroup(context.Context, int64, model.GroupName) (bool, error)
	GetDailyRecipients(context.Context, string) ([]model.DailyRecipient, error)
	HasScheduleSubscription(context.Context, model.ScheduleSubscription) (bool, error)
}

func (r *chatRepository) GetScheduleSubscriptions(ctx context.Context, chatID int64) ([]model.ScheduleSubscription, error) {
	var subscriptions []model.ScheduleSubscription
	err := r.db.WithContext(ctx).Where("chat_id IN (?)", r.scoped().Model(&model.Chat{}).Select("id").Where("id = ?", chatID)).
		Order("id").Find(&subscriptions).Error
	return subscriptions, err
}

// Блокировка строки чата сериализует смену основной группы, подписки и /stop
// как в SQLite, так и в PostgreSQL. Обновляем только id, не время активности.
func (r *chatRepository) lockSubscriptionChat(tx *gorm.DB, chatID int64) (*model.Chat, error) {
	query := tx.Model(&model.Chat{}).Where("id = ?", chatID)
	if r.platform != "" {
		query = query.Where("platform = ?", r.platform)
	}
	result := query.UpdateColumn("id", chatID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var chat model.Chat
	err := tx.First(&chat, chatID).Error
	return &chat, err
}

func insertSubscription(tx *gorm.DB, chatID int64, group model.Group) (bool, error) {
	subscription := model.ScheduleSubscription{ChatID: chatID, GroupName: group.GroupName, DepartmentName: group.DepartmentName}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chat_id"}, {Name: "group_name"}}, DoNothing: true}).Create(&subscription)
	return result.RowsAffected != 0, result.Error
}

func (r *chatRepository) AddScheduleSubscription(ctx context.Context, chatID int64, group model.Group) (added bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.lockSubscriptionChat(tx, chatID); err != nil {
			return err
		}
		var err error
		added, err = insertSubscription(tx, chatID, group)
		if err != nil {
			return err
		}
		return tx.Model(&model.Chat{}).Where("id = ?", chatID).Update("state", model.ChatStateDefault).Error
	})
	return
}

func (r *chatRepository) SetPrimaryGroup(ctx context.Context, chatID int64, group model.Group) (chat *model.Chat, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		chat, err = r.lockSubscriptionChat(tx, chatID)
		if err != nil {
			return err
		}
		if chat.GroupName != nil && *chat.GroupName != group.GroupName {
			if err := tx.Where("chat_id = ? AND group_name = ?", chatID, *chat.GroupName).Delete(&model.ScheduleSubscription{}).Error; err != nil {
				return err
			}
		}
		if _, err := insertSubscription(tx, chatID, group); err != nil {
			return err
		}
		if err := tx.Model(&model.Chat{}).Where("id = ?", chatID).Updates(map[string]any{
			"group": group.GroupName, "department": group.DepartmentName, "state": model.ChatStateDefault,
		}).Error; err != nil {
			return err
		}
		return tx.First(chat, chatID).Error
	})
	return
}

func (r *chatRepository) RemoveScheduleSubscription(ctx context.Context, chatID, subscriptionID int64) (removed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		chat, err := r.lockSubscriptionChat(tx, chatID)
		if err != nil {
			return err
		}
		var subscription model.ScheduleSubscription
		err = tx.Where("id = ? AND chat_id = ?", subscriptionID, chatID).First(&subscription).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if chat.GroupName != nil && *chat.GroupName == subscription.GroupName {
			return ErrPrimarySubscription
		}
		result := tx.Delete(&subscription)
		removed = result.RowsAffected != 0
		if result.Error != nil {
			return result.Error
		}
		return disableEmptyDaily(tx, chatID)
	})
	return
}

// Исчезновение одной группы не отключает ежедневную рассылку остальных.
func (r *chatRepository) RemoveUnavailableGroup(ctx context.Context, chatID int64, group model.GroupName) (removed bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		chat, err := r.lockSubscriptionChat(tx, chatID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result := tx.Where("chat_id = ? AND group_name = ?", chatID, group).Delete(&model.ScheduleSubscription{})
		if result.Error != nil {
			return result.Error
		}
		removed = result.RowsAffected != 0
		if chat.GroupName != nil && *chat.GroupName == group {
			removed = true
			if err := tx.Model(&model.Chat{}).Where("id = ?", chatID).Updates(map[string]any{
				"group": nil, "department": nil, "pair_sending": false,
				"update_notification": false, "state": model.ChatStateDefault,
			}).Error; err != nil {
				return err
			}
		}
		return disableEmptyDaily(tx, chatID)
	})
	return
}

func disableEmptyDaily(tx *gorm.DB, chatID int64) error {
	return tx.Model(&model.Chat{}).Where("id = ?", chatID).
		Where("NOT EXISTS (?)", tx.Model(&model.ScheduleSubscription{}).Select("1").Where("chat_id = ?", chatID)).
		Update("daily_sending_time", nil).Error
}

func (r *chatRepository) GetDailyRecipients(ctx context.Context, dailyTime string) ([]model.DailyRecipient, error) {
	var chats []model.Chat
	if err := r.scoped().WithContext(ctx).Where("daily_sending_time = ?", dailyTime).Order("id").Find(&chats).Error; err != nil {
		return nil, err
	}
	if len(chats) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(chats))
	byID := make(map[int64]model.Chat, len(chats))
	for _, chat := range chats {
		ids = append(ids, chat.ID)
		byID[chat.ID] = chat
	}
	var subscriptions []model.ScheduleSubscription
	if err := r.db.WithContext(ctx).Where("chat_id IN ?", ids).Order("id").Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	recipients := make([]model.DailyRecipient, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		recipients = append(recipients, model.DailyRecipient{Chat: byID[subscription.ChatID], Subscription: subscription})
	}
	return recipients, nil
}

func (r *chatRepository) HasScheduleSubscription(ctx context.Context, subscription model.ScheduleSubscription) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ScheduleSubscription{}).
		Where("id = ? AND chat_id = ? AND group_name = ?", subscription.ID, subscription.ChatID, subscription.GroupName).
		Where("chat_id IN (?)", r.scoped().Model(&model.Chat{}).Select("id")).Count(&count).Error
	return count != 0, err
}
