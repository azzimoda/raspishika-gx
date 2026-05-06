package mainbot

import (
	"fmt"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
)

func Test_commandMatchFunction(t *testing.T) {
	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	messageUpdate := func(text string, chatType models.ChatType) *models.Update {
		return &models.Update{Message: &models.Message{Chat: models.Chat{Type: chatType}, Text: text}}
	}

	tests := []struct {
		// Named input parameters for target function.
		pattern  string
		username string
		// Named input parameters for generated function.
		updateMessage *models.Update
		want          bool
	}{
		// Private chat
		// - simple command
		{"start", "mybot", messageUpdate("/start", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/wrong", models.ChatTypePrivate), false},
		{"start", "mybot", messageUpdate("/start args", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/start args\nmore args", models.ChatTypePrivate), true},

		// - command with username
		{"start", "mybot", messageUpdate("/start@mybot", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/wrong@mybot", models.ChatTypePrivate), false},
		{"start", "mybot", messageUpdate("/start@mybot args", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/start@mybot args\nmore args", models.ChatTypePrivate), true},

		// - command with other username
		{"start", "mybot", messageUpdate("/start@otherbot", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/wrong@otherbot", models.ChatTypePrivate), false},
		{"start", "mybot", messageUpdate("/start@otherbot args", models.ChatTypePrivate), true},
		{"start", "mybot", messageUpdate("/start@otherbot args\nmore args", models.ChatTypePrivate), true},

		// Group chat
		// - simple command
		{"start", "mybot", messageUpdate("/start", models.ChatTypeGroup), true},
		{"start", "mybot", messageUpdate("/wrong", models.ChatTypeGroup), false},
		{"start", "mybot", messageUpdate("/start args", models.ChatTypeGroup), true},
		{"start", "mybot", messageUpdate("/start args\nmore args", models.ChatTypeGroup), true},

		// - command with my username
		{"start", "mybot", messageUpdate("/start@mybot", models.ChatTypeGroup), true},
		{"start", "mybot", messageUpdate("/wrong@mybot", models.ChatTypeGroup), false},
		{"start", "mybot", messageUpdate("/start@mybot args", models.ChatTypeGroup), true},
		{"start", "mybot", messageUpdate("/start@mybot args\nmore args", models.ChatTypeGroup), true},

		// - command with other username
		{"start", "mybot", messageUpdate("/start@otherbot", models.ChatTypeGroup), false},
		{"start", "mybot", messageUpdate("/wrong@otherbot", models.ChatTypeGroup), false},
		{"start", "mybot", messageUpdate("/start@otherbot args", models.ChatTypeGroup), false},
		{"start", "mybot", messageUpdate("/start@otherbot args\nmore args", models.ChatTypeGroup), false},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s-%s-%s-%s-%t",
			tt.pattern, tt.username, tt.updateMessage.Message.Chat.Type, tt.updateMessage.Message.Text, tt.want)

		t.Run(name, func(t *testing.T) {
			got := commandMatchFunc(tt.pattern, tt.username)(tt.updateMessage)
			if got != tt.want {
				t.Errorf("commandMatchFunction() = %v, want %v", got, tt.want)
			}
		})
	}
}
