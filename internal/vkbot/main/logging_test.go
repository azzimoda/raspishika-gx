package vkbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
)

func TestHandleLogsTextMessage(t *testing.T) {
	b, m, c, _ := testBot()
	st := withStats(b)
	addChat(c, 2000000001, model.ChatAccessAll)
	run(t, b, incoming(2000000001, 10, "/start"))
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.Kind != "message" || entry.Data != "/start" || entry.MessageID != 1 || entry.ChatID != 2000000001 {
		t.Fatalf("bad entry: %+v", entry)
	}
	if entry.Error == nil || *entry.Error != "" {
		t.Fatalf("expected empty error on success, got %v", entry.Error)
	}
	if entry.IsCached || entry.GroupOrTeacher != "" {
		t.Fatalf("unexpected metadata: %+v", entry)
	}
	if m.adminChecks != 0 {
		t.Fatal("start should not require admin")
	}
}

func TestHandleLogsKeyboardPayload(t *testing.T) {
	b, _, c, _ := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	msg := incoming(10, 10, "")
	msg.Payload = `{"command":"settings"}`
	run(t, b, msg)
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.Kind != "keyboard" || entry.Data != `{"command":"settings"}` {
		t.Fatalf("bad keyboard entry: %+v", entry)
	}
}

func TestHandleLogsScheduledUpdateMetadata(t *testing.T) {
	b, _, c, s := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	s.schedule = &model.ScheduleData{
		IsOld: true,
		Days:  []model.ScheduleDay{{Date: "05.09.2026", Pairs: []model.Pair{{Kind: model.PairKindSubject, Discipline: "Математика"}}}},
	}
	run(t, b, incoming(10, 10, "/today"))
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.GroupOrTeacher != "ИСПт-22-(9)-2" || !entry.IsCached {
		t.Fatalf("schedule metadata not logged: %+v", entry)
	}
}

func TestHandleLogsHandlerError(t *testing.T) {
	b, _, c, s := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	s.err = errors.New("schedule service down")
	b.schedules = s
	expected := "schedule service down"
	if err := b.Handle(context.Background(), incoming(10, 10, "/today")); err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected %q error, got %v", expected, err)
	}
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.Error == nil || *entry.Error == "" {
		t.Fatalf("expected non-empty error, got %v", entry.Error)
	}
	if !entry.IsOk() {
		return
	}
	t.Fatalf("error entry marked OK: %+v", entry)
}

func TestHandleSkipsIgnoredAndUnhandledUpdates(t *testing.T) {
	cases := []struct {
		name string
		msg  func() vkclient.Message
	}{
		{"outgoing", func() vkclient.Message {
			m := incoming(10, 10, "/start")
			m.Out = true
			return m
		}},
		{"conversation chatter", func() vkclient.Message {
			return incoming(2000000001, 11, "Обсудим это после пары")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, _, c, _ := testBot()
			st := withStats(b)
			addChat(c, 2000000001, model.ChatAccessAdminOnly)
			run(t, b, tc.msg())
			if len(st.entries) != 0 {
				t.Fatalf("ignored update was logged: %+v", st.entries)
			}
		})
	}
}

func TestHandleSkipsUnhandledPrivateText(t *testing.T) {
	b, _, c, _ := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	run(t, b, incoming(10, 10, "Просто разговор ни о чём"))
	if len(st.entries) != 0 {
		t.Fatalf("unhandled private message was logged: %+v", st.entries)
	}
}

func TestHandleLogsStopCommand(t *testing.T) {
	b, _, c, _ := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	run(t, b, incoming(10, 10, "/stop"))
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry for /stop, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.Kind != "message" || entry.Data != "/stop" {
		t.Fatalf("bad stop entry: %+v", entry)
	}
}

func TestHandleLogsTeacherScheduleMetadata(t *testing.T) {
	b, _, c, s := testBot()
	st := withStats(b)
	addChat(c, 10, model.ChatAccessAll)
	s.teachers = []model.Teacher{{TeacherID: "7", Name: "Иванов И.И."}}
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{{Date: "05.09.2026"}}}
	run(t, b, incoming(10, 10, "/teacher Иванов"))
	if len(st.entries) != 1 {
		t.Fatalf("expected 1 update log entry, got %d", len(st.entries))
	}
	entry := st.entries[0]
	if entry.GroupOrTeacher != "Иванов И.И." {
		t.Fatalf("teacher metadata not logged: %+v", entry)
	}
}

func TestHandleStatsWriteFailureIsWarnedButNotFatal(t *testing.T) {
	b, _, c, _ := testBot()
	st := withStats(b)
	st.err = fmt.Errorf("db down")
	addChat(c, 10, model.ChatAccessAll)
	run(t, b, incoming(10, 10, "/start"))
	if len(st.entries) != 1 {
		t.Fatalf("expected statistics attempt, got %d", len(st.entries))
	}
}
