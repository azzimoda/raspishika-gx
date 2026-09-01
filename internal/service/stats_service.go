package service

import (
	"context"
	"fmt"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func NewStatsService(logRepo repository.LogRepository, chatRepo repository.ChatRepository) *StatsService {
	return &StatsService{
		logRepo:  logRepo,
		chatRepo: chatRepo,
	}
}

type StatsService struct {
	logRepo  repository.LogRepository
	chatRepo repository.ChatRepository
}

func (s *StatsService) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	return s.logRepo.LogUpdate(ctx, log)
}
func (s *StatsService) LogBroadcastTask(ctx context.Context, log *model.BroadcastTaskLog) error {
	return s.logRepo.LogBroadcastTask(ctx, log)
}
func (s *StatsService) UpdateBroadcastTaskLog(ctx context.Context, log *model.BroadcastTaskLog) error {
	return s.logRepo.UpdateBroadcastTaskLog(ctx, log)
}
func (s *StatsService) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	return s.logRepo.LogBroadcast(ctx, log)
}

func (s *StatsService) GetChatStats(ctx context.Context, start, end time.Time) (*ChatStatsData, error) {
	chatsTotal, err := s.chatRepo.CountAllChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count all chats: %w", err)
	}
	chatsPrivate, err := s.chatRepo.CountPricateChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count private chats: %w", err)
	}
	chatActivities, err := s.chatRepo.CountChatActivitiesByPeriod(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to count chats by activity: %w", err)
	}
	chatsNew, err := s.chatRepo.CountNewChatsByPeriod(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to count new chats: %w", err)
	}
	chatsNewGrouped, err := s.chatRepo.GetNewChatCountByYearByPeriod(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get new chat count by group: %w", err)
	}
	chatsPerGroup, err := s.chatRepo.GetAvgChatPerGroup(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get avg chat per group: %w", err)
	}
	groupsTotal, err := s.chatRepo.CountAllConfiguredGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count all configured groups: %w", err)
	}
	chatsByDepartment, err := s.chatRepo.GetChatCountByDepartment(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat count by department: %w", err)
	}
	chatsByAccess, err := s.chatRepo.GetChatsByAccessLevel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get chats by access level: %w", err)
	}
	topGroups, err := s.chatRepo.GetTopGroupsByChatCount(ctx, topGroupsLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top groups by chat count: %w", err)
	}

	stats := &ChatStatsData{
		ChatsTotal:      chatsTotal,
		ChatsPrivate:    chatsPrivate,
		ChatsActive:     chatActivities.Active,
		ChatsSemiactive: chatActivities.Semiactive,
		ChatsInactive:   chatActivities.Inactive,
		ChatsNew:        chatsNew,
		ChatsNewGrouped: chatsNewGrouped,
		ChatsPerGroup:   chatsPerGroup,
		GroupsTotal:     groupsTotal,
		Departments:     chatsByDepartment,
		TopGroups:       topGroups,
		ChatsByAccess:   chatsByAccess,
	}
	return stats, nil
}

// topGroupsLimit bounds the number of rows in the "top groups by chats" table.
const topGroupsLimit = 10

type ChatStatsData struct {
	ChatsTotal      int                           `json:"chats_total"`
	ChatsPrivate    int                           `json:"chats_private"`
	ChatsActive     int                           `json:"chats_active"`
	ChatsSemiactive int                           `json:"chats_semiactive"`
	ChatsInactive   int                           `json:"chats_inactive"`
	ChatsNew        int                           `json:"chats_new"`
	ChatsNewGrouped map[int]int                   `json:"chats_new_grouped"`
	ChatsPerGroup   float64                       `json:"chat_per_group"`
	GroupsTotal     int                           `json:"groups_total"`
	Departments     []repository.NameCount        `json:"departments"`
	TopGroups       []repository.NameCount        `json:"top_groups"`
	ChatsByAccess   map[model.ChatAccessLevel]int `json:"chats_by_access"`
}

func (s *StatsService) GetConfigStats(ctx context.Context) (*ConfigStatsData, error) {
	chatsTotal, err := s.chatRepo.CountAllChats(ctx)
	if err != nil {
		return nil, err
	}
	chatsWithConfiguredGroup, err := s.chatRepo.CountChatsWithConfiguredGroup(ctx)
	if err != nil {
		return nil, err
	}
	uniqueConfiguredGroups, err := s.chatRepo.CountUniqueConfiguredGroups(ctx)
	if err != nil {
		return nil, err
	}
	dailyEnabled, err := s.chatRepo.CountDailyEnabled(ctx)
	if err != nil {
		return nil, err
	}
	pairEnabled, err := s.chatRepo.CountPairEnabled(ctx)
	if err != nil {
		return nil, err
	}
	changeEnabled, err := s.chatRepo.CountChangeEnabled(ctx)
	if err != nil {
		return nil, err
	}
	darkEnabled, err := s.chatRepo.CountDarkEnabled(ctx)
	if err != nil {
		return nil, err
	}
	chatCountByTime, err := s.chatRepo.GetGroupedCountChatCountByTime(ctx)
	if err != nil {
		return nil, err
	}
	privateChatsConfigured, err := s.chatRepo.CountPrivateChatsWithConfiguredGroup(ctx)
	if err != nil {
		return nil, err
	}
	watchedGroupNames, err := s.chatRepo.GetWatchedGroupNames(ctx)
	if err != nil {
		return nil, err
	}

	return &ConfigStatsData{
		ChatsTotal:             chatsTotal,
		ConfiguredGroupsTotal:  chatsWithConfiguredGroup,
		ConfiguredGroupsUnique: uniqueConfiguredGroups,
		DailyEnabled:           dailyEnabled,
		PairEnabled:            pairEnabled,
		ChangeEnabled:          changeEnabled,
		DarkEnabled:            darkEnabled,
		ChatCountByTime:        chatCountByTime,
		PrivateChatsConfigured: privateChatsConfigured,
		WatchedGroups:          len(watchedGroupNames),
	}, nil
}

type ConfigStatsData struct {
	ChatsTotal             int
	ConfiguredGroupsTotal  int
	ConfiguredGroupsUnique int
	DailyEnabled           int
	PairEnabled            int
	ChangeEnabled          int
	DarkEnabled            int
	ChatCountByTime        []repository.TimeCount
	PrivateChatsConfigured int
	WatchedGroups          int
}

func (s *StatsService) GetLogStats(ctx context.Context, start, end time.Time) (*LogStatsData, error) {
	updatesTotal, err := s.logRepo.CountUpdateLogsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	updatesSuccess, err := s.logRepo.CountSuccessfulUpdateLogsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}

	broadcastTasks, err := s.logRepo.CountBroadcastTaskLogsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	broadcastLogs, err := s.logRepo.CountBroadcastLogsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	broadcastSuccess, err := s.logRepo.CountSuccessfulBroadcastLogsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	broadcastDaily, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BDaily, start, end)
	if err != nil {
		return nil, err
	}
	broadcastPair, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BPair, start, end)
	if err != nil {
		return nil, err
	}
	broadcastChange, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BChange, start, end)
	if err != nil {
		return nil, err
	}

	requestsActual, err := s.logRepo.CountActualRequests(ctx, start, end)
	if err != nil {
		return nil, err
	}
	requestsPotential, err := s.logRepo.CountPotentialRequests(ctx, start, end)
	if err != nil {
		return nil, err
	}

	requestsCached, requestsUncached, err := s.logRepo.CountScheduleRequestsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	distinctChats, err := s.logRepo.CountDistinctChatsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	updatesByKind, err := s.logRepo.GetUpdateLogCountByKind(ctx, start, end)
	if err != nil {
		return nil, err
	}
	updateLatency, err := s.logRepo.GetUpdateLatencyStatsByPeriod(ctx, start, end)
	if err != nil {
		return nil, err
	}
	requestsByHour, err := s.logRepo.GetRequestsCountByHour(ctx, start, end)
	if err != nil {
		return nil, err
	}
	topRequestedSchedules, err := s.logRepo.GetTopRequestedSchedules(ctx, start, end, topSchedulesLimit)
	if err != nil {
		return nil, err
	}
	broadcastTasksByKind, err := s.logRepo.GetBroadcastTaskStatsByKind(ctx, start, end)
	if err != nil {
		return nil, err
	}

	scheduleRequests := requestsCached + requestsUncached
	cacheHitRate := 0.0
	if scheduleRequests > 0 {
		cacheHitRate = float64(requestsCached) / float64(scheduleRequests)
	}

	stats := &LogStatsData{
		UpdatesTotal:   updatesTotal,
		UpdatesSuccess: updatesSuccess,

		BroadcastTasks:   broadcastTasks,
		BroadcastLogs:    broadcastLogs,
		BroadcastSuccess: broadcastSuccess,
		BroadcastDaily:   broadcastDaily,
		BroadcastPair:    broadcastPair,
		BroadcastChange:  broadcastChange,

		RequestsActual:    requestsActual,
		RequestsPotential: requestsPotential,

		ScheduleRequests:      scheduleRequests,
		RequestsCached:        requestsCached,
		RequestsUncached:      requestsUncached,
		CacheHitRate:          cacheHitRate,
		DistinctChats:         distinctChats,
		UpdatesByKind:         updatesByKind,
		UpdateLatency:         refutil.DerefOrTypeDefault(updateLatency),
		RequestsByHour:        requestsByHour,
		TopRequestedSchedules: topRequestedSchedules,
		BroadcastByKind:       broadcastTasksByKind,
	}
	return stats, nil
}

// topSchedulesLimit bounds the number of rows in the "top requested schedules"
// table.
const topSchedulesLimit = 10

type LogStatsData struct {
	UpdatesTotal   int
	UpdatesSuccess int

	BroadcastTasks   int
	BroadcastLogs    int
	BroadcastSuccess int
	BroadcastDaily   int
	BroadcastPair    int
	BroadcastChange  int

	RequestsActual    int
	RequestsPotential int

	ScheduleRequests      int
	RequestsCached        int
	RequestsUncached      int
	CacheHitRate          float64
	DistinctChats         int
	UpdatesByKind         map[string]int
	UpdateLatency         repository.LatencyStats
	RequestsByHour        []repository.TimeCount
	TopRequestedSchedules []repository.NameCount
	BroadcastByKind       []repository.BroadcastTaskKindStats
}

func (s *StatsService) GetGeneralStats(ctx context.Context, start, end time.Time) (*GeneralStatsData, error) {
	chatStats, err := s.GetChatStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	logStats, err := s.GetLogStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return &GeneralStatsData{ChatStatsData: chatStats, LogStatsData: logStats}, nil
}

type GeneralStatsData struct {
	*ChatStatsData
	*LogStatsData
}

func (s *StatsService) HealthCheck() error {
	now := time.Now()
	past := now.Add(-time.Hour)
	if _, err := s.GetGeneralStats(context.Background(), past, now); err != nil {
		return err
	}
	return nil
}
