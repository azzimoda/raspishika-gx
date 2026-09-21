package messenger

import (
	"context"

	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
)

// VK adapts the VK community bot client to the Messenger interface used by the
// broadcast service. Texts pass through PlainText because VK has no HTML; the
// schedule navigation options render as a native VK keyboard.
type VK struct {
	client *vkclient.Client
}

// NewVK wraps a VK community client as a Messenger.
func NewVK(client *vkclient.Client) *VK {
	return &VK{client: client}
}

func (v *VK) SendMessagePeer(ctx context.Context, peerID int64, text string, opts ...SendOptions) (int, error) {
	return v.client.SendMessage(ctx, peerID, PlainText(text), keyboardFromOpts(opts))
}

func (v *VK) SendPhotoPeer(ctx context.Context, peerID int64, filename string, data []byte, caption string, opts ...SendOptions) error {
	_, err := v.client.SendPhoto(ctx, peerID, filename, data, PlainText(caption), keyboardFromOpts(opts))
	return err
}

func keyboardFromOpts(opts []SendOptions) *vkbotutil.Keyboard {
	for _, o := range opts {
		if o.Buttons != nil {
			return vkbotutil.ScheduleKeyboard(o.Buttons.Value, o.Buttons.Days, o.Buttons.CurrentIdx, o.Buttons.LinkURL)
		}
	}
	return nil
}

func (v *VK) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return v.client.DeleteMessage(ctx, chatID, messageID)
}

func (v *VK) IsForbidden(err error) bool {
	return vkclient.IsForbidden(err)
}

func (v *VK) IsAdmin(ctx context.Context, peerID, userID int64) (bool, error) {
	return v.client.IsAdmin(ctx, peerID, userID)
}
