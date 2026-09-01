package adminbot

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/go-telegram/bot/models"
)

func TestBlockHelpersMarshal(t *testing.T) {
	cases := []struct {
		name  string
		block models.InputRichBlock
		want  string
	}{
		{"heading", blockHeading("H", 1), `"type":"heading"`},
		{"paragraph", blockParagraph(textPlain("p")), `"type":"paragraph"`},
		{"divider", blockDivider(), `"type":"divider"`},
		{"table", blockTable([]string{"A", "B"}, [][]string{{"1", "2"}}, true, true), `"type":"table"`},
		{"details", blockDetails("s", false, blockParagraph(textPlain("x"))), `"type":"details"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := json.Marshal(c.block)
			if err != nil {
				t.Fatalf("marshal %s: %v", c.name, err)
			}
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("missing type %q in %s", c.want, out)
			}
		})
	}
}

func TestBlockTableCellsAlign(t *testing.T) {
	block := blockTable([]string{"H"}, [][]string{{"v"}}, false, false)
	out, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"align":"center"`) || !strings.Contains(s, `"align":"left"`) {
		t.Fatalf("expected header/data aligns in %s", s)
	}
	if !strings.Contains(s, `"is_header":true`) {
		t.Fatalf("expected header cell in %s", s)
	}
}

func TestTextLabelValueIsSequence(t *testing.T) {
	rt := textLabelValue("Total", "42")
	out, err := json.Marshal(rt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "[") {
		t.Fatalf("expected rich text sequence, got %s", s)
	}
	if !strings.Contains(s, `"type":"bold"`) {
		t.Fatalf("expected bold label in %s", s)
	}
	if !strings.Contains(s, `"42"`) {
		t.Fatalf("expected plain value in %s", s)
	}
}

func TestBuildDashboard(t *testing.T) {
	general := &service.GeneralStatsData{
		ChatStatsData: &service.ChatStatsData{
			ChatsTotal:      10,
			ChatsPrivate:    8,
			ChatsActive:     5,
			ChatsSemiactive: 2,
			ChatsInactive:   3,
			ChatsNew:        1,
			ChatsNewGrouped: map[int]int{2023: 4, 0: 1},
			ChatsPerGroup:   2.5,
			GroupsTotal:     4,
			Departments:     []repository.NameCount{{Name: "МПК", Count: 6}},
			TopGroups:       []repository.NameCount{{Name: "ИВТ-1", Count: 3}},
			ChatsByAccess:   map[model.ChatAccessLevel]int{model.ChatAccessAll: 7},
		},
		LogStatsData: &service.LogStatsData{
			UpdatesTotal:   100,
			UpdatesSuccess: 95,
			BroadcastTasks: 20, BroadcastLogs: 200, BroadcastSuccess: 190,
			BroadcastDaily: 100, BroadcastPair: 60, BroadcastChange: 40,
			RequestsActual: 50, RequestsPotential: 200,
			ScheduleRequests: 80, RequestsCached: 60, RequestsUncached: 20,
			DistinctChats:         6,
			UpdatesByKind:         map[string]int{"message": 70, "callback_query": 30},
			UpdateLatency:         repository.LatencyStats{Count: 100},
			RequestsByHour:        []repository.TimeCount{{Time: "12", Count: 30}},
			TopRequestedSchedules: []repository.NameCount{{Name: "ИВТ-1", Count: 40}},
			BroadcastByKind: []repository.BroadcastTaskKindStats{{
				Kind: model.BDaily, Tasks: 10, Groups: 100, AvgElapsedMs: 1500,
			}},
		},
	}
	config := &service.ConfigStatsData{
		ChatsTotal: 10, ConfiguredGroupsTotal: 8, ConfiguredGroupsUnique: 4,
		DailyEnabled: 8, PairEnabled: 7, ChangeEnabled: 2, DarkEnabled: 2,
		ChatCountByTime:        []repository.TimeCount{{Time: "08:00", Count: 3}},
		PrivateChatsConfigured: 7,
		WatchedGroups:          2,
	}

	spec := periodSpec{start: time.Now().Add(-24 * time.Hour), end: time.Now(), isRelative: true}
	blocks := buildDashboard(general, config, spec)
	if len(blocks) < 10 {
		t.Fatalf("expected many blocks, got %d", len(blocks))
	}

	out, err := json.Marshal(models.InputRichMessage{Blocks: blocks})
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}
	s := string(out)
	for _, want := range []string{`"type":"heading"`, `"type":"paragraph"`, `"type":"divider"`,
		`"type":"details"`, `"type":"table"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in dashboard", want)
		}
	}
}

func TestFormattingHelpers(t *testing.T) {
	if got := percent(10, 3); got != "30.0%" {
		t.Fatalf("percent(10,3) = %q", got)
	}
	if got := percent(0, 0); got != "0.0%" {
		t.Fatalf("percent(0,0) = %q", got)
	}
	if got := formatDuration(48 * time.Hour); got != "2d" {
		t.Fatalf("formatDuration(48h) = %q", got)
	}
	if got := latencyLabel(repository.LatencyStats{Count: 0}); got != "no data" {
		t.Fatalf("latencyLabel(empty) = %q", got)
	}
	if got := latencyLabel(repository.LatencyStats{Count: 3, AvgMs: 10, P95Ms: 50, MaxMs: 200}); got != "avg/p95/max 10/50/200 ms" {
		t.Fatalf("latencyLabel = %q", got)
	}
}
