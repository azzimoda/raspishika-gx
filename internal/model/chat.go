package model

import (
	"time"

	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
)

type ChatID int64

func (i ChatID) Int64() int64 { return int64(i) }

// ChatPeerOffset is the boundary between private chats and group conversations
// for platforms that encode both in a single integer namespace (VK). Private
// peers are less than the offset, group conversations are greater or equal.
const ChatPeerOffset int64 = 2000000000

// IsPrivate reports whether the peer is a private chat. Telegram group chats
// are negative, VK group conversations start at ChatPeerOffset, so a positive
// value below the offset is a private chat on both platforms.
func (i ChatID) IsPrivate() bool {
	v := int64(i)
	return v > 0 && v < ChatPeerOffset
}

type ChatState string

// Platform is the messenger a chat belongs to. Chats of different platforms
// share the single chats table and are scoped by this discriminator.
type Platform string

const (
	PlatformTelegram Platform = "telegram"
	PlatformVK       Platform = "vk"
)

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
	PeerID           ChatID          `gorm:"column:tg_chat_id"`
	Platform         Platform        `gorm:"column:platform"`
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

func (c *Chat) IsPrivate() bool { return c.PeerID.IsPrivate() }

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
