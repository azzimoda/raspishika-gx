// Package messenger defines the platform-neutral outbound transport used by
// services for scheduled and administrator broadcasts.
package messenger

import (
	"context"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

// SendOptions carries per-send additions such as navigation buttons. Options
// are optional: calls without them behave like plain messages.
type SendOptions struct {
	// Buttons, when set, asks the adapter to render schedule navigation
	// buttons under the message. The payload is platform-neutral — the adapter
	// maps it to its native buttons (a Telegram inline keyboard, a VK keyboard
	// or a plain-text approximation).
	Buttons *ScheduleButtons
}

// ScheduleButtons describes schedule-navigation buttons in a
// messenger-agnostic way.
type ScheduleButtons struct {
	// Value is the group name or teacher id embedded into the callbacks.
	Value string
	// Days are the schedule days in image/table order.
	Days []model.ScheduleDay
	// CurrentIdx is the day to highlight for a day-text layout, or -1 for the
	// full-week photo layout (no day highlighted).
	CurrentIdx int
	// LinkURL is the schedule page opened by the link button (may be empty).
	LinkURL string
}

// Messenger is the platform-neutral transport. Text and captions passed in
// are Telegram HTML; each adapter renders them however the target platform
// supports it (Telegram HTML, a VK plain-text approximation...). Adapters
// form their own native buttons and keyboards, so the interface carries no
// messenger-specific markup.
type Messenger interface {
	// SendMessagePeer sends an HTML message to the given peer and returns its
	// platform message ID so the caller can later DeleteMessage it.
	SendMessagePeer(ctx context.Context, peerID int64, text string, opts ...SendOptions) (int, error)
	// SendPhotoPeer sends a photo with an HTML caption to the given peer.
	SendPhotoPeer(ctx context.Context, peerID int64, filename string, data []byte, caption string, opts ...SendOptions) error
	// DeleteMessage deletes a previously sent message.
	DeleteMessage(ctx context.Context, chatID int64, messageID int) error
	// IsForbidden reports whether err is a permanent refusal of the peer
	// (kicked bot, closed conversation) rather than a transient failure.
	IsForbidden(err error) bool
	// IsAdmin reports whether userID is a peer administrator on the platform.
	IsAdmin(ctx context.Context, peerID, userID int64) (bool, error)
}
