// Package messenger defines the platform-neutral outbound transport used by
// services for scheduled and administrator broadcasts.
package messenger

import "context"

// Messenger is the platform-neutral transport. Text and captions passed in
// are Telegram HTML; each adapter renders them however the target platform
// supports it (Telegram HTML, a VK plain-text approximation...). Adapters
// form their own native buttons and keyboards, so the interface carries no
// messenger-specific markup.
type Messenger interface {
	// SendMessagePeer sends an HTML message to the given peer.
	SendMessagePeer(ctx context.Context, peerID int64, text string) error
	// SendPhotoPeer sends a photo with an HTML caption to the given peer.
	SendPhotoPeer(ctx context.Context, peerID int64, filename string, data []byte, caption string) error
	// DeleteMessage deletes a previously sent message.
	DeleteMessage(ctx context.Context, chatID int64, messageID int) error
	// IsForbidden reports whether err is a permanent refusal of the peer
	// (kicked bot, closed conversation) rather than a transient failure.
	IsForbidden(err error) bool
	// IsAdmin reports whether userID is a peer administrator on the platform.
	IsAdmin(ctx context.Context, peerID, userID int64) (bool, error)
}
