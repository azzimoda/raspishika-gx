package botutil

import (
	"encoding/base64"
	"strings"
)

func NewStartCommand(ele ...string) *StartCommand { return &StartCommand{elements: ele} }
func ParseStartCommand(text string) *StartCommand {
	return &StartCommand{elements: strings.Split(text, "-")}
}

type StartCommand struct{ elements []string }

func (c *StartCommand) String() string {
	encoded := make([]string, 0, len(c.elements))
	for _, e := range c.elements {
		encoded = append(encoded, base64.RawURLEncoding.EncodeToString([]byte(e)))
	}
	return strings.Join(encoded, "-")
}

func (c *StartCommand) Arg(idx int) string {
	if idx < 0 || idx >= len(c.elements) {
		return ""
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(c.elements[idx])
	return string(decoded)
}
