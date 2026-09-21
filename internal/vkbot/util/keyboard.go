// Package vkbotutil builds VK keyboards, menus and their paging used by the
// interactive VK bot.
package vkbotutil

import "encoding/json"

// Keyboard mirrors the VK API keyboard object. Text buttons deliver the
// command embedded in the message payload as message_new events.
type Keyboard struct {
	OneTime bool       `json:"one_time"`
	Inline  bool       `json:"inline,omitempty"`
	Buttons [][]Button `json:"buttons"`
}

// Button is a single VK keyboard button.
type Button struct {
	Action ButtonAction `json:"action"`
	Color  string       `json:"color,omitempty"`
}

// ButtonAction is the VK button action descriptor.
type ButtonAction struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Payload string `json:"payload,omitempty"`
	Link    string `json:"link,omitempty"`
}

// JSON encodes the keyboard for the messages.send "keyboard" parameter.
func (k *Keyboard) JSON() (string, error) {
	if k == nil {
		return "", nil
	}
	data, err := json.Marshal(k)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// TextButton produces a button that sends a message carrying the command
// inside the message payload.
func TextButton(label, command string) Button {
	payload, _ := json.Marshal(struct {
		Command string `json:"command"`
	}{command})
	return Button{Action: ButtonAction{Type: "text", Label: label, Payload: string(payload)}, Color: "primary"}
}

// LinkButton produces a button that opens an external link.
func LinkButton(label, link string) Button {
	return Button{Action: ButtonAction{Type: "open_link", Label: label, Link: link}}
}
