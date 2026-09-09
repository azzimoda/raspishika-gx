package model

import "time"

// ScheduleSubscription связывает чат с группой ежедневной рассылки.
type ScheduleSubscription struct {
	ID             int64     `gorm:"primaryKey"`
	ChatID         int64     `gorm:"column:chat_id;uniqueIndex:idx_subscription_chat_group"`
	GroupName      GroupName `gorm:"column:group_name;uniqueIndex:idx_subscription_chat_group"`
	DepartmentName string    `gorm:"column:department_name"`
	CreatedAt      time.Time
}

// DailyRecipient хранит конкретную подписку и настройки чата на момент выборки.
type DailyRecipient struct {
	Chat         Chat
	Subscription ScheduleSubscription
}
