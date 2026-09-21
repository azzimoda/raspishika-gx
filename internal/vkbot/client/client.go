// Package vkclient wraps SevereCloud/vksdk into the shapes the schedule bot
// needs: Bots Long Poll events, message sending with keyboards and photos, and
// conversation admin checks.
package vkclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/api/params"
	"github.com/SevereCloud/vksdk/v3/events"
	"github.com/SevereCloud/vksdk/v3/longpoll-bot"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
	"github.com/rs/zerolog/log"
)

// DefaultVersion is the VK API version used when the config does not specify one.
const DefaultVersion = "5.199"

// ChatPeerOffset separates group chats from private conversations in VK peer IDs.
const ChatPeerOffset int64 = 2000000000

// Message is the trimmed view of a message_new event the bot handler consumes.
type Message struct {
	ID                    int
	PeerID, FromID        int64
	Text, Payload         string
	ConversationMessageID int
	Out                   bool
}

// Client owns the community VK API and the Bots Long Poll loop. Outbound calls
// are safe for concurrent use; Run must have a single caller.
type Client struct {
	vk      *api.VK
	groupID int64
}

// New validates the community token and group. It does not contact VK.
func New(token string, groupID int64, version string) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("VK community token is empty")
	}
	if groupID <= 0 {
		return nil, errors.New("VK community ID must be positive")
	}
	if version == "" {
		version = DefaultVersion
	}
	vk := api.NewVK(token)
	vk.Version = version
	return &Client{vk: vk, groupID: groupID}, nil
}

// IsForbidden identifies destinations which no longer permit bot messages.
// Other failures (configuration, flood limits, transient errors) are
// deliberately not treated as an unsubscribe request.
func IsForbidden(err error) bool {
	var apiErr api.Error
	if errors.As(err, &apiErr) {
		switch int(apiErr.Code) {
		case 901, 902, 917:
			return true
		}
	}
	return false
}

func pause(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// randomID returns a positive int for messages.send random_id.
func randomID() int {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return int(time.Now().UnixNano()) & 0x7fffffff
	}
	return int(binary.BigEndian.Uint32(b[:])%2147483647) + 1
}

// splitText cuts at UTF-16 units (VK's message length base) keeping a margin
// below the platform limit.
func splitText(text string) []string {
	if text == "" {
		return []string{""}
	}
	const maxUnits = 4000
	parts := []string{}
	start, units, newline := 0, 0, -1
	for i, r := range text {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > maxUnits {
			cut := i
			if newline >= start && newline-start > (i-start)/2 {
				cut = newline + 1
			}
			parts = append(parts, text[start:cut])
			start, units, newline = cut, 0, -1
			for _, old := range text[cut:i] {
				units++
				if old > 0xffff {
					units++
				}
			}
		}
		units += width
		if r == '\n' {
			newline = i
		}
	}
	if start < len(text) {
		parts = append(parts, text[start:])
	}
	return parts
}

// SendMessage splits long texts and sends each part in order; the keyboard and
// optional attachment accompany the final part. It returns the last message ID.
func (c *Client) SendMessage(ctx context.Context, peerID int64, text string, keyboard *vkbotutil.Keyboard) (int, error) {
	return c.send(ctx, peerID, text, "", keyboard)
}

// SendPhoto uploads data through the community's messages upload server and
// sends it with a caption. It returns the message ID.
func (c *Client) SendPhoto(ctx context.Context, peerID int64, filename string, data []byte, caption string, keyboard *vkbotutil.Keyboard) (int, error) {
	if len(data) == 0 {
		return 0, errors.New("VK photo is empty")
	}
	photos, err := c.vk.UploadMessagesPhoto(int(peerID), bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("upload VK photo: %w", err)
	}
	if len(photos) == 0 {
		return 0, errors.New("VK saved photo is missing")
	}
	return c.send(ctx, peerID, caption, photos[0].ToAttachment(), keyboard)
}

func (c *Client) send(ctx context.Context, peerID int64, text, attachment string, keyboard *vkbotutil.Keyboard) (lastID int, err error) {
	if peerID == 0 {
		return 0, errors.New("VK destination peer_id is zero")
	}
	if text == "" && attachment == "" {
		return 0, errors.New("VK message is empty")
	}
	parts := splitText(text)
	for i, part := range parts {
		b := params.NewMessagesSendBuilder()
		b.Message(part)
		b.RandomID(randomID())
		b.PeerID(int(peerID))
		if attachment != "" {
			b.Attachment(attachment)
		}
		if i == len(parts)-1 && keyboard != nil {
			encoded, encodeErr := keyboard.JSON()
			if encodeErr != nil {
				return lastID, fmt.Errorf("encode VK keyboard: %w", encodeErr)
			}
			if encoded != "" {
				b.Keyboard(encoded)
			}
		}
		id, sendErr := c.vk.MessagesSend(b.Params.WithContext(ctx))
		if sendErr != nil {
			return lastID, sendErr
		}
		lastID = id
	}
	return lastID, nil
}

// DeleteMessage deletes one or more messages in the peer conversation.
func (c *Client) DeleteMessage(ctx context.Context, peerID int64, messageIDs ...int) error {
	if len(messageIDs) == 0 {
		return nil
	}
	b := params.NewMessagesDeleteBuilder()
	b.MessageIDs(messageIDs)
	b.PeerID(int(peerID))
	_, err := c.vk.MessagesDelete(b.Params.WithContext(ctx))
	return err
}

// IsAdmin reports whether userID administers the given peer. Private
// conversations belong to their owner; group chats use the conversation
// membership roles.
func (c *Client) IsAdmin(ctx context.Context, peerID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if peerID < ChatPeerOffset {
		return peerID == userID, nil
	}
	params := api.Params{
		"peer_id": int(peerID),
		"count":   1000,
	}.WithContext(ctx)
	resp, err := c.vk.MessagesGetConversationMembers(params)
	if err != nil {
		return false, err
	}
	for _, member := range resp.Items {
		if int64(member.MemberID) == userID {
			return bool(member.IsOwner) || bool(member.IsAdmin), nil
		}
	}
	return false, nil
}

// isFatalInitError reports Long Poll bootstrap failures that a retry cannot fix.
func isFatalInitError(err error) bool {
	var apiErr api.Error
	if errors.As(err, &apiErr) {
		switch int(apiErr.Code) {
		case 5, 15, 27, 100:
			return true
		}
	}
	return false
}

// Run processes incoming message_new events sequentially so conversation setup
// stays ordered. A bounded in-memory event cache prevents duplicate deliveries
// during reconnects. Handler errors are logged and do not stop other users'
// messages; handlers must send their own user-facing error response. Context
// cancellation stops promptly.
func (c *Client) Run(ctx context.Context, handler func(context.Context, Message) error) error {
	if handler == nil {
		return errors.New("VK message handler is nil")
	}
	backoff := time.Second
	seen, order := map[string]struct{}{}, []string{}
	for ctx.Err() == nil {
		lp, err := longpoll.NewLongPoll(c.vk, int(c.groupID))
		if err != nil {
			if isFatalInitError(err) {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn().Err(err).Msg("VK Long Poll init failed; reconnecting")
			if err := pause(ctx, backoff); err != nil {
				return ctx.Err()
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {
			msg := fromMessage(obj)
			if msg.PeerID == 0 || msg.FromID <= 0 || msg.Out {
				return
			}
			key := events.EventIDFromContext(ctx)
			if key == "" && msg.ConversationMessageID != 0 {
				key = fmt.Sprintf("%d:c%d", msg.PeerID, msg.ConversationMessageID)
			}
			if key == "" && msg.ID != 0 {
				key = fmt.Sprintf("%d:m%d", msg.PeerID, msg.ID)
			}
			if key != "" {
				if _, exists := seen[key]; exists {
					return
				}
				seen[key] = struct{}{}
				order = append(order, key)
				if len(order) > 4096 {
					delete(seen, order[0])
					order = order[1:]
				}
			}
			if ctx.Err() != nil {
				return
			}
			if err := handler(ctx, msg); err != nil && ctx.Err() == nil {
				log.Warn().Err(err).Int64("peer", msg.PeerID).Msg("VK handler failed")
			}
		})
		if err := lp.RunWithContext(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn().Err(err).Msg("VK Long Poll session failed; reconnecting")
			if err := pause(ctx, backoff); err != nil {
				return ctx.Err()
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
	}
	return ctx.Err()
}

func fromMessage(obj events.MessageNewObject) Message {
	return Message{
		ID:                    obj.Message.ID,
		PeerID:                int64(obj.Message.PeerID),
		FromID:                int64(obj.Message.FromID),
		Text:                  obj.Message.Text,
		Payload:               obj.Message.Payload,
		ConversationMessageID: obj.Message.ConversationMessageID,
		Out:                   bool(obj.Message.Out),
	}
}
