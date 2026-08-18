package model_test

import (
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/testutil"
	"github.com/spf13/viper"
)

func TestChatID_IsPrivate(t *testing.T) {
	tests := []struct {
		name   string // description of this test case
		chatID model.ChatID
		want   bool
	}{
		{
			name:   "positive chat ID is private, returns true",
			chatID: model.ChatID(1),
			want:   true,
		},
		{
			name:   "negative chat ID is not private, returns false",
			chatID: model.ChatID(-1),
			want:   false,
		},
		{
			name:   "zero chat ID returns false",
			chatID: model.ChatID(0),
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.chatID.IsPrivate()
			if got != tt.want {
				t.Errorf("IsPrivate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChat_GetState(t *testing.T) {
	if err := testutil.MoveToProjectRoot(); err != nil {
		t.Fatal(err)
	}
	config.Init()

	tests := []struct {
		name        string // description of this test case
		chat        model.Chat
		wantState   model.ChatState
		wantExpired bool
	}{
		{
			name: "chat with default state and recently active",
			chat: model.Chat{
				State:     model.ChatStateDefault,
				UpdatedAt: time.Now(),
			},
			wantState:   model.ChatStateDefault,
			wantExpired: false,
		},
		{
			name: "chat with default state and recentry inactive",
			chat: model.Chat{
				State:     model.ChatStateDefault,
				UpdatedAt: time.Now().Add(-2 * viper.GetDuration(config.KeyChatStateTTL)),
			},
			wantState:   model.ChatStateDefault,
			wantExpired: false,
		},
		{
			name: "chat with non-default state and recently active",
			chat: model.Chat{
				State:     model.ChatStateSelectingGroup,
				UpdatedAt: time.Now(),
			},
			wantState:   model.ChatStateSelectingGroup,
			wantExpired: false,
		},
		{
			name: "chat with non-default state and recently inactive",
			chat: model.Chat{
				State:     model.ChatStateSelectingGroup,
				UpdatedAt: time.Now().Add(-2 * viper.GetDuration(config.KeyChatStateTTL)),
			},
			wantState:   model.ChatStateDefault,
			wantExpired: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotState, gotExpired := tt.chat.GetState()
			if gotState != tt.wantState {
				t.Errorf("GetState() = %v, want %v", gotState, tt.wantState)
			}
			if gotExpired != tt.wantExpired {
				t.Errorf("GetState() = %v, want %v", gotExpired, tt.wantExpired)
			}
		})
	}
}

func TestChat_WithState(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		state     model.ChatState
		wantState model.ChatState
	}{
		{name: "with default state", state: model.ChatStateDefault, wantState: model.ChatStateDefault},
		{name: "with selecting group state", state: model.ChatStateSelectingGroup, wantState: model.ChatStateSelectingGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c model.Chat
			gotChat := c.WithState(tt.state)
			if gotChat.State != tt.wantState {
				t.Errorf("WithState() = %v, want %v", gotChat.State, tt.wantState)
			}
		})
	}
}
