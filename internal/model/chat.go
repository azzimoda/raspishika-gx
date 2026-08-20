package model

import (
	"time"

	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
)

type ChatID int64

func (i ChatID) Int64() int64    { return int64(i) }
func (i ChatID) IsPrivate() bool { return i > 0 }

type ChatState string

const (
	ChatStateDefault          ChatState = "default"
	ChatStateSelectingGroup   ChatState = "selecting_group"
	ChatStateSelectingTeacher ChatState = "selecting_teacher"
	ChatStateSelectingTime    ChatState = "selecting_time"
)

type ChatAccessLevel int

// ChatAccess constants define the access level of a group chat.
const (
	// All commands are available for all users.
	ChatAccessAll ChatAccessLevel = 0
	// Configuration commands are available only for administrators.
	ChatAccessConfigAdmin ChatAccessLevel = 1
	// All commands are available only for administrators.
	ChatAccessAdminOnly ChatAccessLevel = 2
)

type Chat struct {
	ID               int64           `gorm:"primaryKey;column:id"`
	TgChatID         ChatID          `gorm:"column:tg_chat_id"`
	UserName         *string         `gorm:"column:username"`
	State            ChatState       `gorm:"column:state"`
	DepartmentName   *string         `gorm:"column:department"`
	GroupName        *GroupName      `gorm:"column:group"`
	DailySendingTime *string         `gorm:"column:daily_sending_time"`
	PairSending      bool            `gorm:"column:pair_sending"`
	ChangeAlert      bool            `gorm:"column:update_notification"`
	Access           ChatAccessLevel `gorm:"column:access"`
	DarkMode         bool            `gorm:"column:dark_mode"`
	CreatedAt        time.Time       `gorm:"column:created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at"`
}

func (c *Chat) IsPrivate() bool { return c.TgChatID.IsPrivate() }

// GetState returns actual state of the chat.
//
// If chat's state is not Defeult and chat state TTL is expired, returns (ChatStateDefault, true).
// Otherwise returns actual state and false.
func (c *Chat) GetState() (state ChatState, expired bool) {
	if c.State != ChatStateDefault && time.Since(c.UpdatedAt) >= viper.GetDuration(config.KeyChatStateTTL) {
		c.State = ChatStateDefault
		return ChatStateDefault, true
	}
	return c.State, false
}

// WithState updates chat's state and returns reference to this chat.
func (c *Chat) WithState(state ChatState) *Chat {
	c.State = state
	return c
}
