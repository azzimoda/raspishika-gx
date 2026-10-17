package service

import (
	"context"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/repository"
)

func TestNormalizeErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{
			"retry delay",
			"too many requests, Too Many Requests: retry after 3: retry_after 3",
			"too many requests, Too Many Requests: retry after N: retry after N",
		},
		{"joined duplicates", "boom\nboom", "boom"},
		{"joined causes", "first\nsecond", "first | second"},
		{"whitespace noise", "  boom   \n\t bang  ", "boom | bang"},
		{
			"bot token",
			`error do request for method sendChatAction, Post "https://api.telegram.org/bot123456:ABC/sendChatAction": context canceled`,
			`error do request for method sendChatAction, Post "https://api.telegram.org/bot***/sendChatAction": context canceled`,
		},
		{
			"screenshot file",
			"open storage/screenshots/schedule_Отделение 1_ГСРф-26-(11)-1_dark.png: permission denied",
			"open storage/screenshots/schedule_*.png: permission denied",
		},
		{"long id", "chat 123456789 not found", "chat N not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeErrorMessage(c.in); got != c.want {
				t.Fatalf("normalizeErrorMessage(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMergeErrorCounts(t *testing.T) {
	rows := []repository.NameCount{
		{Name: "retry after 3", Count: 2},
		{Name: "retry after 18", Count: 5},
		{Name: "other", Count: 5},
		{Name: "", Count: 9}, // dropped: empty after normalization
		{Name: "retry_after 19", Count: 1},
	}

	got := mergeErrorCounts(rows, 10)
	if len(got) != 2 {
		t.Fatalf("mergeErrorCounts() = %+v, want 2 rows", got)
	}
	if got[0].Message != "retry after N" || got[0].Count != 8 {
		t.Fatalf("mergeErrorCounts()[0] = %+v, want {retry after N 8}", got[0])
	}
	if got[1].Message != "other" || got[1].Count != 5 {
		t.Fatalf("mergeErrorCounts()[1] = %+v, want {other 5}", got[1])
	}

	limited := mergeErrorCounts(rows, 1)
	if len(limited) != 1 || limited[0].Message != "retry after N" {
		t.Fatalf("mergeErrorCounts(limit 1) = %+v, want only the top row", limited)
	}
}

type errorStatsLogStub struct {
	repository.LogRepository
	updates   []repository.NameCount
	broadcast []repository.NameCount
}

func (s *errorStatsLogStub) GetUpdateErrorStatsByPeriod(context.Context, time.Time, time.Time) ([]repository.NameCount, error) {
	return s.updates, nil
}

func (s *errorStatsLogStub) GetBroadcastErrorStatsByPeriod(context.Context, time.Time, time.Time) ([]repository.NameCount, error) {
	return s.broadcast, nil
}

func TestGetErrorStats(t *testing.T) {
	stub := &errorStatsLogStub{
		updates: []repository.NameCount{
			{Name: "retry after 3", Count: 2},
			{Name: "retry after 9", Count: 3},
			{Name: "boom", Count: 1},
		},
		broadcast: []repository.NameCount{
			{Name: "flood", Count: 4},
		},
	}
	service := NewStatsService(stub, nil)

	stats, err := service.GetErrorStats(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("GetErrorStats() error: %v", err)
	}

	if stats.UpdatesTotal != 6 {
		t.Fatalf("UpdatesTotal = %d, want 6", stats.UpdatesTotal)
	}
	if stats.BroadcastTotal != 4 {
		t.Fatalf("BroadcastTotal = %d, want 4", stats.BroadcastTotal)
	}
	if len(stats.Updates) != 2 {
		t.Fatalf("Updates = %+v, want 2 normalized rows", stats.Updates)
	}
	if stats.Updates[0].Message != "retry after N" || stats.Updates[0].Count != 5 {
		t.Fatalf("Updates[0] = %+v, want {retry after N 5}", stats.Updates[0])
	}
	if len(stats.Broadcast) != 1 || stats.Broadcast[0].Message != "flood" {
		t.Fatalf("Broadcast = %+v, want one flood row", stats.Broadcast)
	}
}
