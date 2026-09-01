package adminbot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/go-telegram/bot/models"
)

// buildDashboard assembles the rich message blocks of the admin dashboard:
// general statistics for the given period plus current configuration stats.
func buildDashboard(general *service.GeneralStatsData, config *service.ConfigStatsData, spec periodSpec) []models.InputRichBlock {
	blocks := []models.InputRichBlock{
		blockHeading("Dashboard", 1),
		blockParagraph(textPlain(fmt.Sprintf("General: %s · Config: current", generalPeriodLabel(spec)))),
		blockDivider(),
	}
	blocks = append(blocks, buildGeneralSection(general)...)
	blocks = append(blocks, blockDivider())
	blocks = append(blocks, buildConfigSection(config, general.ChatsPrivate, general.ChatsByAccess)...)
	return blocks
}

// dashboardExport is the JSON file payload exported from a dashboard: the
// general+config statistics for the given period plus its human label.
type dashboardExport struct {
	Period  string                    `json:"period"`
	General *service.GeneralStatsData `json:"general"`
	Config  *service.ConfigStatsData  `json:"config"`
}

// exportStatsPayload renders the dashboard statistics as an indented JSON file
// payload ready to be sent as a document.
func exportStatsPayload(general *service.GeneralStatsData, config *service.ConfigStatsData, spec periodSpec) ([]byte, error) {
	payload, err := json.MarshalIndent(dashboardExport{
		Period:  generalPeriodLabel(spec),
		General: general,
		Config:  config,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// dashboardExportMarkup builds the inline keyboard attached to the dashboard
// message: a single button that exports the same statistics as a JSON file.
func dashboardExportMarkup(period string) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Экспорт JSON", CallbackData: botutil.CallbackCommandExportStats + "\n" + period}},
		},
	}
}

// exportFilename builds a file name for the dashboard export, e.g.
// "dashboard-2026-09-01-1d.json".
func exportFilename(spec periodSpec) string {
	period := "period"
	if spec.isRelative {
		period = formatDuration(spec.end.Sub(spec.start))
	} else if spec.label != "" {
		period = strings.NewReplacer(" ", "", "→", "-", ":", "").Replace(spec.label)
	}
	return fmt.Sprintf("dashboard-%s-%s.json", time.Now().In(statsTZ).Format("2006-01-02"), period)
}

// generalPeriodLabel renders a human description of the statistics window.
func generalPeriodLabel(spec periodSpec) string {
	if spec.label != "" {
		return spec.label
	}
	if spec.isRelative {
		return fmt.Sprintf("last %s", formatDuration(spec.end.Sub(spec.start)))
	}
	return fmt.Sprintf("%s → %s", formatMomentLocal(spec.start), formatMomentLocal(spec.end))
}

func buildGeneralSection(general *service.GeneralStatsData) []models.InputRichBlock {
	var blocks []models.InputRichBlock

	blocks = append(blocks, blockHeading("Chats", 2))
	blocks = append(blocks,
		blockParagraph(textLabelValue("Total", fmt.Sprintf("%d · Private/Group %d/%d",
			general.ChatsTotal, general.ChatsPrivate, general.ChatsTotal-general.ChatsPrivate))),
		blockParagraph(textLabelValue("Activity", fmt.Sprintf("active/semi/inactive %d/%d/%d",
			general.ChatsActive, general.ChatsSemiactive, general.ChatsInactive))),
		blockParagraph(textLabelValue("New registered", fmt.Sprintf("%d", general.ChatsNew))),
		blockParagraph(textLabelValue("DAU", fmt.Sprintf("%d", general.DistinctChats))),
	)
	if len(general.Departments) > 0 {
		blocks = append(blocks, blockDetails(
			fmt.Sprintf("Chats by department (%d)", len(general.Departments)), false,
			blockTable([]string{"Department", "Chats"}, nameCountRows(general.Departments), true, false),
		))
	}
	if len(general.TopGroups) > 0 {
		blocks = append(blocks, blockDetails(
			fmt.Sprintf("Top groups by chats (%d)", len(general.TopGroups)), false,
			blockTable([]string{"Group", "Chats"}, nameCountRows(general.TopGroups), true, false),
		))
	}
	if len(general.ChatsNewGrouped) > 0 {
		blocks = append(blocks, blockDetails(
			"New chats grouped by year", false,
			blockTable([]string{"Year", "Count"}, yearRows(general.ChatsNewGrouped), true, false),
		))
	}

	blocks = append(blocks, blockHeading("Groups", 2))
	blocks = append(blocks, blockParagraph(textLabelValue("Total", fmt.Sprintf("%d · Chats per group %.2f",
		general.GroupsTotal, general.ChatsPerGroup))))

	blocks = append(blocks, blockHeading("Updates", 2))
	blocks = append(blocks,
		blockParagraph(textLabelValue("Total", fmt.Sprintf("%d · Success %d (%s)",
			general.UpdatesTotal, general.UpdatesSuccess, percent(general.UpdatesTotal, general.UpdatesSuccess)))),
		blockParagraph(textLabelValue("Latency", latencyLabel(general.UpdateLatency))),
	)
	if len(general.UpdatesByKind) > 0 {
		blocks = append(blocks, blockDetails(
			"Updates by kind", false,
			blockTable([]string{"Kind", "Count"}, nameCountRows(kindRows(general.UpdatesByKind)), true, false),
		))
	}

	blocks = append(blocks, blockHeading("Broadcast", 2))
	blocks = append(blocks,
		blockParagraph(textLabelValue("Tasks/Sends", fmt.Sprintf("%d/%d · Success %d (%s)",
			general.BroadcastTasks, general.BroadcastLogs, general.BroadcastSuccess,
			percent(general.BroadcastLogs, general.BroadcastSuccess)))),
		blockParagraph(textLabelValue("Delivered", fmt.Sprintf("daily/pair/change %d/%d/%d",
			general.BroadcastDaily, general.BroadcastPair, general.BroadcastChange))),
	)
	if len(general.BroadcastByKind) > 0 {
		blocks = append(blocks, blockDetails(
			"Broadcast tasks by kind", false,
			blockTable([]string{"Kind", "Tasks", "Groups", "Avg, ms"}, broadcastKindRows(general.BroadcastByKind), true, false),
		))
	}

	blocks = append(blocks, blockHeading("Requests", 2))
	blocks = append(blocks,
		blockParagraph(textLabelValue("Schedule requests", fmt.Sprintf("%d (%s from cache)",
			general.ScheduleRequests, percent(general.ScheduleRequests, general.RequestsCached)))),
		blockParagraph(textLabelValue("Actual/Potential", fmt.Sprintf("%d/%d",
			general.RequestsActual, general.RequestsPotential))),
	)
	if len(general.TopRequestedSchedules) > 0 {
		blocks = append(blocks, blockDetails(
			fmt.Sprintf("Top requested groups/teachers (%d)", len(general.TopRequestedSchedules)), false,
			blockTable([]string{"Group/Teacher", "Requests"}, nameCountRows(general.TopRequestedSchedules), true, false),
		))
	}
	if len(general.RequestsByHour) > 0 {
		blocks = append(blocks, blockDetails(
			"Requests by hour", false,
			blockTable([]string{"Hour", "Requests"}, timeCountRows(general.RequestsByHour), true, false),
		))
	}

	return blocks
}

func buildConfigSection(config *service.ConfigStatsData, privateChatsTotal int, chatsByAccess map[model.ChatAccessLevel]int) []models.InputRichBlock {
	var blocks []models.InputRichBlock

	blocks = append(blocks, blockHeading("Settings", 1))
	blocks = append(blocks,
		blockParagraph(textLabelValue("Total chats", fmt.Sprintf("%d · Configured %d (%s)",
			config.ChatsTotal, config.ConfiguredGroupsTotal, percent(config.ChatsTotal, config.ConfiguredGroupsTotal)))),
		blockParagraph(textLabelValue("Unique groups", fmt.Sprintf("%d · Watched groups %d",
			config.ConfiguredGroupsUnique, config.WatchedGroups))),
		blockParagraph(textLabelValue("Features", fmt.Sprintf("daily/pair/change %d/%d/%d",
			config.DailyEnabled, config.PairEnabled, config.ChangeEnabled))),
		blockParagraph(textLabelValue("Dark theme", fmt.Sprintf("%d (%s)",
			config.DarkEnabled, percent(config.ChatsTotal, config.DarkEnabled)))),
		blockParagraph(textLabelValue("Onboarded private chats", fmt.Sprintf("%d/%d (%s)",
			config.PrivateChatsConfigured, privateChatsTotal, percent(privateChatsTotal, config.PrivateChatsConfigured)))),
	)
	if len(chatsByAccess) > 0 {
		blocks = append(blocks, blockDetails(
			"Chats by access level", false,
			blockTable([]string{"Access", "Chats"}, accessRows(chatsByAccess), true, false),
		))
	}
	if len(config.ChatCountByTime) > 0 {
		blocks = append(blocks, blockDetails(
			"Chat count by time", false,
			blockTable([]string{"Time", "Chats"}, timeCountRows(config.ChatCountByTime), true, false),
		))
	}

	return blocks
}

// ---- small formatting helpers ----

func percent(total, part int) string {
	if total <= 0 {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)/float64(total)*100)
}

func formatDuration(d time.Duration) string {
	switch {
	case d%(365*24*time.Hour) == 0:
		return fmt.Sprintf("%dy", d/(365*24*time.Hour))
	case d%(30*24*time.Hour) == 0:
		return fmt.Sprintf("%dm", d/(30*24*time.Hour))
	case d%(7*24*time.Hour) == 0:
		return fmt.Sprintf("%dw", d/(7*24*time.Hour))
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	default:
		return d.String()
	}
}

func latencyLabel(latency repository.LatencyStats) string {
	if latency.Count <= 0 {
		return "no data"
	}
	return fmt.Sprintf("avg/p95/max %d/%d/%d ms", latency.AvgMs, latency.P95Ms, latency.MaxMs)
}

func broadcastKindRows(rows []repository.BroadcastTaskKindStats) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, []string{broadcastKindLabel(row.Kind), fmt.Sprintf("%d", row.Tasks),
			fmt.Sprintf("%d", row.Groups), fmt.Sprintf("%d", row.AvgElapsedMs)})
	}
	return out
}

func broadcastKindLabel(kind model.BroadcastKind) string {
	switch kind {
	case model.BDaily:
		return "daily"
	case model.BPair:
		return "pair"
	case model.BChange:
		return "change"
	case model.BMass:
		return "mass"
	default:
		return "all"
	}
}

func kindRows(kinds map[string]int) []repository.NameCount {
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]repository.NameCount, 0, len(names))
	for _, name := range names {
		rows = append(rows, repository.NameCount{Name: name, Count: kinds[name]})
	}
	return rows
}

func nameCountRows(items []repository.NameCount) [][]string {
	out := make([][]string, 0, len(items))
	for _, item := range items {
		out = append(out, []string{item.Name, fmt.Sprintf("%d", item.Count)})
	}
	return out
}

func timeCountRows(items []repository.TimeCount) [][]string {
	out := make([][]string, 0, len(items))
	for _, item := range items {
		out = append(out, []string{item.Time, fmt.Sprintf("%d", item.Count)})
	}
	return out
}

func yearRows(years map[int]int) [][]string {
	keys := make([]int, 0, len(years))
	for year := range years {
		keys = append(keys, year)
	}
	sort.Ints(keys)
	out := make([][]string, 0, len(keys))
	for _, year := range keys {
		label := "none"
		if year != 0 {
			label = fmt.Sprintf("%d", year)
		}
		out = append(out, []string{label, fmt.Sprintf("%d", years[year])})
	}
	return out
}

func accessRows(access map[model.ChatAccessLevel]int) [][]string {
	levels := []model.ChatAccessLevel{
		model.ChatAccessAll,
		model.ChatAccessConfigAdmin,
		model.ChatAccessAdminOnly,
	}
	out := make([][]string, 0, len(levels))
	for _, level := range levels {
		out = append(out, []string{accessLevelLabel(level), fmt.Sprintf("%d", access[level])})
	}
	return out
}

func accessLevelLabel(level model.ChatAccessLevel) string {
	switch level {
	case model.ChatAccessAll:
		return "all"
	case model.ChatAccessConfigAdmin:
		return "config_admin"
	case model.ChatAccessAdminOnly:
		return "admin_only"
	default:
		return fmt.Sprintf("level_%d", int(level))
	}
}

// ---- rich message block helpers ----

func textPlain(s string) models.RichText {
	return models.RichText{PlainText: s}
}

func textBold(s string) models.RichText {
	return models.RichText{
		Type:         models.RichTextTypeBold,
		RichTextBold: &models.RichTextBold{Type: models.RichTextTypeBold, Text: textPlain(s)},
	}
}

// textLabelValue is a rich text fragment rendered as a bold label followed by
// a plain value, e.g. "Total: 42".
func textLabelValue(label, value string) models.RichText {
	return models.RichText{Array: []models.RichText{textBold(label + ": "), textPlain(value)}}
}

func blockParagraph(rt models.RichText) models.InputRichBlock {
	return models.InputRichBlock{
		Type:                    models.RichBlockTypeParagraph,
		InputRichBlockParagraph: &models.InputRichBlockParagraph{Text: rt},
	}
}

func blockHeading(text string, size int) models.InputRichBlock {
	return models.InputRichBlock{
		Type: models.RichBlockTypeSectionHeading,
		InputRichBlockSectionHeading: &models.InputRichBlockSectionHeading{
			Type: models.RichBlockTypeSectionHeading, Text: textPlain(text), Size: size,
		},
	}
}

func blockDivider() models.InputRichBlock {
	return models.InputRichBlock{Type: models.RichBlockTypeDivider, InputRichBlockDivider: &models.InputRichBlockDivider{}}
}

func blockDetails(summary string, isOpen bool, blocks ...models.InputRichBlock) models.InputRichBlock {
	return models.InputRichBlock{
		Type: models.RichBlockTypeDetails,
		InputRichBlockDetails: &models.InputRichBlockDetails{
			Type: models.RichBlockTypeDetails, Summary: textPlain(summary), Blocks: blocks, IsOpen: isOpen,
		},
	}
}

func blockTable(header []string, rows [][]string, bordered, striped bool) models.InputRichBlock {
	limit := 100
	if striped {
		limit = 12
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	cells := make([][]models.RichBlockTableCell, 0, len(rows)+1)
	if len(header) > 0 {
		headerCells := make([]models.RichBlockTableCell, 0, len(header))
		for _, h := range header {
			headerCells = append(headerCells, models.RichBlockTableCell{
				Text: &models.RichText{PlainText: h}, IsHeader: true, Align: "center", Valign: "middle",
			})
		}
		cells = append(cells, headerCells)
	}
	for _, row := range rows {
		cellsRow := make([]models.RichBlockTableCell, 0, len(row))
		for _, v := range row {
			cellsRow = append(cellsRow, models.RichBlockTableCell{
				Text: &models.RichText{PlainText: v}, Align: "left", Valign: "top",
			})
		}
		cells = append(cells, cellsRow)
	}
	return models.InputRichBlock{
		Type: models.RichBlockTypeTable,
		InputRichBlockTable: &models.InputRichBlockTable{
			Type: models.RichBlockTypeTable, Cells: cells, IsBordered: bordered, IsStriped: striped,
		},
	}
}
