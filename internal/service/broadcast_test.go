package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/messenger"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

var (
	errTestForbidden = errors.New("test forbidden")
	errTestTransient = errors.New("test transient")
)

type broadcastDelivery struct {
	peerID   int64
	text     string
	filename string
	data     []byte
	buttons  *messenger.ScheduleButtons
}

type broadcastDeletion struct {
	peerID    int64
	messageID int
}

type broadcastMessengerStub struct {
	mu                  sync.Mutex
	deliveries          []broadcastDelivery
	deleted             []broadcastDeletion
	sendError           error
	started             chan struct{}
	release             chan struct{}
	waitForCancellation bool
}

func (m *broadcastMessengerStub) SendMessagePeer(
	ctx context.Context,
	peerID int64,
	text string,
	opts ...messenger.SendOptions,
) (int, error) {
	m.mu.Lock()
	m.deliveries = append(m.deliveries, broadcastDelivery{peerID: peerID, text: text, buttons: firstButtons(opts)})
	messageID := len(m.deliveries)
	m.mu.Unlock()
	if m.started != nil {
		m.started <- struct{}{}
	}
	if m.release != nil {
		select {
		case <-m.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if m.waitForCancellation {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return messageID, m.sendError
}

func (m *broadcastMessengerStub) SendPhotoPeer(
	_ context.Context,
	peerID int64,
	filename string,
	data []byte,
	caption string,
	opts ...messenger.SendOptions,
) error {
	m.deliveries = append(m.deliveries, broadcastDelivery{
		peerID:   peerID,
		text:     caption,
		filename: filename,
		data:     data,
		buttons:  firstButtons(opts),
	})
	return m.sendError
}

// firstButtons returns the first non-nil schedule buttons from the send options.
func firstButtons(opts []messenger.SendOptions) *messenger.ScheduleButtons {
	for _, o := range opts {
		if o.Buttons != nil {
			return o.Buttons
		}
	}
	return nil
}

func (m *broadcastMessengerStub) DeleteMessage(_ context.Context, peerID int64, messageID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, broadcastDeletion{peerID: peerID, messageID: messageID})
	return nil
}

func (m *broadcastMessengerStub) IsForbidden(err error) bool { return errors.Is(err, errTestForbidden) }

func (m *broadcastMessengerStub) IsAdmin(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}

type broadcastLogsStub struct {
	repository.LogRepository
	logs           []model.BroadcastLog
	tasks          []model.BroadcastTaskLog
	finished       int
	canceledWrites int
}

func (r *broadcastLogsStub) LogBroadcastTask(_ context.Context, task *model.BroadcastTaskLog) error {
	task.ID = int64(len(r.tasks) + 1)
	r.tasks = append(r.tasks, *task)
	return nil
}

func (r *broadcastLogsStub) UpdateBroadcastTaskLog(ctx context.Context, _ *model.BroadcastTaskLog) error {
	if ctx.Err() != nil {
		r.canceledWrites++
	}
	r.finished++
	return nil
}

func (r *broadcastLogsStub) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	if ctx.Err() != nil {
		r.canceledWrites++
	}
	r.logs = append(r.logs, log)
	return nil
}

type broadcastChatsStub struct {
	repository.ChatRepository
	mu          sync.Mutex
	records     map[int64]model.Chat
	lookupError error
	updated     []model.Chat
}

func (r *broadcastChatsStub) UpdateChat(_ context.Context, chat *model.Chat) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updated = append(r.updated, *chat)
	if r.records == nil {
		r.records = make(map[int64]model.Chat)
	}
	r.records[chat.ID] = *chat
	return nil
}

func (r *broadcastChatsStub) GetChat(_ context.Context, id int64) (*model.Chat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lookupError != nil {
		return nil, r.lookupError
	}
	chat, found := r.records[id]
	if !found {
		return nil, gorm.ErrRecordNotFound
	}
	return &chat, nil
}

func (r *broadcastChatsStub) put(chat *model.Chat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[int64]model.Chat)
	}
	r.records[chat.ID] = *chat
}

func (r *broadcastChatsStub) remove(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, id)
}

func newBroadcastFixture(
	t *testing.T, messenger *broadcastMessengerStub,
) (
	*BroadcastService, *broadcastChatsStub, *broadcastLogsStub,
) {
	t.Helper()
	chats, logs := &broadcastChatsStub{}, &broadcastLogsStub{}
	service := NewBroadcastService(messenger, &Services{
		Chat: NewChatService(chats), Stats: NewStatsService(logs, chats),
	}, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		service.Stop(ctx)
	})
	return service, chats, logs
}

func TestBroadcastPairUsesScheduleGroupAndActualDate(t *testing.T) {
	for _, date := range []string{"01.09.2026", "2026-09-01"} {
		t.Run(date, func(t *testing.T) {
			messenger := &broadcastMessengerStub{}
			service, chats, logs := newBroadcastFixture(t, messenger)
			group := model.GroupName("ИСПт-22-(9)-2")
			chat := &model.Chat{ID: 17, PeerID: 2000000009, GroupName: &group, PairSending: true}
			chats.put(chat)
			pair := model.Pair{
				Kind:       model.PairKindSubject,
				Number:     1,
				StartTime:  "08:00",
				EndTime:    "09:30",
				Discipline: "Математика",
				Classroom:  "215",
				Teacher:    "Иванов",
			}
			schedule := &model.ScheduleData{
				Config: model.GroupScheduleConfig(&model.Group{GroupName: group}, false),
				Days: []model.ScheduleDay{
					{Date: "31.08.2026"},
					{Date: date, Pairs: []model.Pair{pair}},
				},
			}
			now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
			err := service.sendPairNotificatins(context.Background(), 3,
				[]*model.ScheduleData{nil, schedule}, map[model.GroupName][]*model.Chat{group: {chat}},
				[]model.GroupName{"unrelated"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(messenger.deliveries) != 1 {
				t.Fatalf("deliveries = %d, want 1", len(messenger.deliveries))
			}
			got := messenger.deliveries[0]
			if got.peerID != 2000000009 || !strings.Contains(got.text, "Математика") {
				t.Fatalf("incorrect delivery: %+v", got)
			}
			if len(logs.logs) != 1 || logs.logs[0].ChatID != chat.ID || logs.logs[0].TaskID != 3 {
				t.Fatalf("delivery log must reference database chat ID: %+v", logs.logs)
			}
			// A stale or future schedule must not produce a reminder today.
			if err := service.sendPairNotificatins(
				context.Background(),
				4,
				[]*model.ScheduleData{schedule},
				map[model.GroupName][]*model.Chat{group: {chat}},
				nil,
				now.AddDate(0, 0, 1),
			); err != nil {
				t.Fatal(err)
			}
			if len(messenger.deliveries) != 1 {
				t.Fatal("sent a reminder for the wrong date")
			}
		})
	}
}

func TestBroadcastForbiddenDisablesSubscriptionsAndPreservesChat(t *testing.T) {
	for _, err := range []error{errTestForbidden, errTestTransient} {
		t.Run(err.Error(), func(t *testing.T) {
			service, chats, logs := newBroadcastFixture(t, &broadcastMessengerStub{})
			group, daily := model.GroupName("ИСПт-22-(9)-2"), "19:00"
			chat := &model.Chat{
				ID:               18,
				PeerID:           101,
				GroupName:        &group,
				DailySendingTime: &daily,
				PairSending:      true,
				ChangeAlert:      true,
				DarkMode:         true,
			}
			chats.put(chat)
			service.recordSend(context.Background(), 9, chat, err)
			if errors.Is(err, errTestTransient) {
				if len(chats.updated) != 0 || !chat.PairSending {
					t.Fatal("temporary failure disabled subscriptions")
				}
			} else {
				if len(chats.updated) != 1 {
					t.Fatalf("subscription updates = %d", len(chats.updated))
				}
				current, lookupErr := chats.GetChat(context.Background(), chat.ID)
				if lookupErr != nil {
					t.Fatal(lookupErr)
				}
				if current.PairSending || current.ChangeAlert || current.DailySendingTime != nil {
					t.Fatal("forbidden recipient still subscribed")
				}
				if current.ID != 18 || current.GroupName == nil || !current.DarkMode {
					t.Fatal("recipient preferences were lost")
				}
			}
			if len(logs.logs) != 1 || logs.logs[0].Error == nil {
				t.Fatal("failed send not logged")
			}
		})
	}
}

func TestBroadcastSchedulePhotoCaptionKeepsHTMLAndStaleWarning(t *testing.T) {
	messenger := &broadcastMessengerStub{}
	service, _, _ := newBroadcastFixture(t, messenger)
	image := &broadcastImage{
		schedule: model.ScheduleData{
			Config: model.GroupScheduleConfig(&model.Group{
				GroupName:    "ИСПт-22-(9)-2",
				GroupID:      "205",
				DepartmentID: "15",
			}, true),
			IsOld: true,
		},
		filename: "schedule.png", data: []byte("image"),
	}
	if err := service.sendSchedule(context.Background(), &model.Chat{PeerID: 123}, image); err != nil {
		t.Fatal(err)
	}
	got := messenger.deliveries[0]
	if got.peerID != 123 || got.filename != "schedule.png" || string(got.data) != "image" {
		t.Fatalf("photo = %+v", got)
	}
	if !strings.Contains(got.text, "<i>") || !strings.Contains(got.text, "неактуальной") {
		t.Fatalf("caption = %s", got.text)
	}
	if got.buttons == nil {
		t.Fatal("schedule photo must carry navigation buttons")
	}
	if got.buttons.Value != "ИСПт-22-(9)-2" || got.buttons.CurrentIdx != -1 || got.buttons.LinkURL == "" {
		t.Fatalf("navigation buttons = %+v", got.buttons)
	}
}

func TestBroadcastMassShutdownCancelsSendAndRejectsNewJobs(t *testing.T) {
	messenger := &broadcastMessengerStub{started: make(chan struct{}, 1), waitForCancellation: true}
	service, chats, logs := newBroadcastFixture(t, messenger)
	chat := &model.Chat{ID: 31, PeerID: 900}
	chats.put(chat)
	if err := service.BroadcastText(
		context.Background(), []*model.Chat{nil, chat, chat}, "<b>Привет</b> &amp; мир",
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-messenger.started:
	case <-time.After(time.Second):
		t.Fatal("send never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	service.Stop(ctx)
	if ctx.Err() != nil {
		t.Fatal("shutdown did not cancel in-flight sending")
	}
	if len(messenger.deliveries) != 1 || messenger.deliveries[0].text != "<b>Привет</b> &amp; мир" {
		t.Fatalf("unexpected sends: %+v", messenger.deliveries)
	}
	if logs.finished != 1 || len(logs.logs) != 1 {
		t.Fatal("mass broadcast task was not finalized")
	}
	if logs.canceledWrites != 0 {
		t.Fatal("audit writes used canceled delivery context")
	}
	if err := service.BroadcastText(context.Background(), []*model.Chat{chat}, "late"); err == nil {
		t.Fatal("broadcast accepted after Stop")
	}
	if service.runJob(func() { t.Error("ran a job after shutdown") }) {
		t.Fatal("job accepted after shutdown")
	}
}

type broadcastAPIStub struct{ APIClient }

func (broadcastAPIStub) GetGroup(_ context.Context, name string) (*model.Group, error) {
	if name == "removed" {
		return nil, apiclient.ErrNotFound
	}
	return &model.Group{GroupName: model.GroupName(name)}, nil
}

func TestBroadcastResetsRemovedGroupAlongsideValidRecipients(t *testing.T) {
	messenger := &broadcastMessengerStub{}
	service, chats, _ := newBroadcastFixture(t, messenger)
	service.Schedule = NewScheduleService(broadcastAPIStub{}, nil, nil)
	valid, removed, daily := model.GroupName("valid"), model.GroupName("removed"), "19:00"
	good := &model.Chat{ID: 1, PeerID: 11, GroupName: &valid}
	bad := &model.Chat{ID: 2, PeerID: 12, GroupName: &removed,
		PairSending: true, ChangeAlert: true, DailySendingTime: &daily}
	chats.put(good)
	chats.put(bad)
	_, groups, _, invalid, stop := service.prepareBroadcast(context.Background(), []*model.Chat{nil, good, bad})
	if stop || len(groups) != 1 || len(invalid) != 1 {
		t.Fatalf("unexpected prepared groups: %v, %v, stop=%v", groups, invalid, stop)
	}
	service.notifyAndResetInvalidChats(context.Background(), invalid)
	if len(messenger.deliveries) != 1 || messenger.deliveries[0].peerID != 12 || len(chats.updated) != 1 {
		t.Fatal("removed recipient was not notified and reset")
	}
	current, err := chats.GetChat(context.Background(), bad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.GroupName != nil || current.PairSending || current.ChangeAlert || current.DailySendingTime != nil {
		t.Fatal("removed group subscriptions not reset")
	}
	if good.GroupName == nil {
		t.Fatal("valid recipient changed")
	}
}

func TestBroadcastCronTimesAndCancellation(t *testing.T) {
	service := NewBroadcastService(nil, nil, nil)
	defer service.Stop(context.Background())
	if err := service.scheduleDaily(service.ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.schedulePairNotification(service.ctx); err != nil {
		t.Fatal(err)
	}
	entries := service.cron.Entries()
	if len(entries) != 8 {
		t.Fatalf("cron entries = %d, want 8", len(entries))
	}
	saturday := time.Date(2026, 9, 5, 19, 0, 0, 0, time.UTC)
	for _, entry := range entries[1:] {
		if next := entry.Schedule.Next(saturday); next.Weekday() != time.Monday {
			t.Fatalf("pair scheduled on %s", next)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepContext(canceled, time.Hour) {
		t.Fatal("canceled sleep returned true")
	}
	if err := service.Run(canceled, BroadcastConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run canceled: %v", err)
	}
}

func TestBroadcastJobGroupConcurrentWaiters(t *testing.T) {
	var jobs jobGroup
	jobs.add()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { jobs.wait(ctx); done <- struct{}{} }()
	}
	jobs.close()
	jobs.done()
	for range 2 {
		<-done
	}
	if ctx.Err() != nil {
		t.Fatal("a concurrent waiter was left blocked")
	}
	if jobs.add() {
		t.Fatal("closed group accepted job")
	}
}

func TestBroadcastQueuedStopSkipsDeletedRecipient(t *testing.T) {
	messenger := &broadcastMessengerStub{started: make(chan struct{}, 2), release: make(chan struct{})}
	service, chats, logs := newBroadcastFixture(t, messenger)
	first := &model.Chat{ID: 1, PeerID: 101}
	second := &model.Chat{ID: 2, PeerID: 102}
	chats.put(first)
	chats.put(second)
	if err := service.BroadcastText(context.Background(), []*model.Chat{first, second}, "Объявление"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-messenger.started:
	case <-time.After(time.Second):
		t.Fatal("first send never started")
	}
	// The second user sends /stop while the first request is in flight.
	chats.remove(second.ID)
	close(messenger.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	service.jobs.wait(ctx)
	if ctx.Err() != nil {
		t.Fatal("broadcast did not complete")
	}
	if len(messenger.deliveries) != 1 || messenger.deliveries[0].peerID != first.PeerID.Int64() {
		t.Fatalf("stopped recipient was sent queued content: %+v", messenger.deliveries)
	}
	if len(logs.logs) != 1 || logs.finished != 1 {
		t.Fatal("skipped recipient logged as delivered or task not finalized")
	}
}

func TestBroadcastQueuedPairOptOutIsHonored(t *testing.T) {
	messenger := &broadcastMessengerStub{started: make(chan struct{}, 2), release: make(chan struct{})}
	service, chats, _ := newBroadcastFixture(t, messenger)
	group := model.GroupName("ИСПт-22-(9)-2")
	first := &model.Chat{ID: 1, PeerID: 101, GroupName: &group, PairSending: true}
	second := &model.Chat{ID: 2, PeerID: 102, GroupName: &group, PairSending: true}
	chats.put(first)
	chats.put(second)
	schedule := &model.ScheduleData{
		Config: model.GroupScheduleConfig(&model.Group{GroupName: group}, false),
		Days: []model.ScheduleDay{{
			Date: "01.09.2026",
			Pairs: []model.Pair{{
				Kind:       model.PairKindSubject,
				Number:     1,
				StartTime:  "08:00",
				EndTime:    "09:30",
				Discipline: "Математика",
			}},
		}},
	}
	done := make(chan error, 1)
	go func() {
		done <- service.sendPairNotificatins(
			context.Background(),
			1,
			[]*model.ScheduleData{schedule},
			map[model.GroupName][]*model.Chat{group: {first, second}},
			nil,
			time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
		)
	}()
	select {
	case <-messenger.started:
	case <-time.After(time.Second):
		t.Fatal("first reminder never started")
	}
	updated := *second
	updated.PairSending = false
	chats.put(&updated)
	close(messenger.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reminders did not complete")
	}
	if len(messenger.deliveries) != 1 {
		t.Fatalf("opted-out recipient received a queued reminder: %+v", messenger.deliveries)
	}
}

func TestBroadcastPairNotificationAutoDelete(t *testing.T) {
	prev := viper.Get(config.KeyPairNotificationTTL)
	viper.Set(config.KeyPairNotificationTTL, 150*time.Millisecond)
	t.Cleanup(func() {
		if prev == nil {
			viper.Set(config.KeyPairNotificationTTL, nil)
		} else {
			viper.Set(config.KeyPairNotificationTTL, prev)
		}
	})

	messenger := &broadcastMessengerStub{}
	service, chats, _ := newBroadcastFixture(t, messenger)
	group := model.GroupName("ИСПт-22-(9)-2")
	chat := &model.Chat{ID: 21, PeerID: 2000000011, GroupName: &group, PairSending: true}
	chats.put(chat)
	pair := model.Pair{
		Kind:       model.PairKindSubject,
		Number:     1,
		StartTime:  "08:00",
		EndTime:    "09:30",
		Discipline: "Математика",
		Classroom:  "215",
	}
	schedule := &model.ScheduleData{
		Config: model.GroupScheduleConfig(&model.Group{GroupName: group}, false),
		Days:   []model.ScheduleDay{{Date: "01.09.2026", Pairs: []model.Pair{pair}}},
	}
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	if err := service.sendPairNotificatins(context.Background(), 7, []*model.ScheduleData{schedule},
		map[model.GroupName][]*model.Chat{group: {chat}}, nil, now); err != nil {
		t.Fatal(err)
	}
	if len(messenger.deliveries) != 1 || len(messenger.deliveries[0].text) == 0 {
		t.Fatalf("pair reminder not delivered: %+v", messenger.deliveries)
	}
	if len(messenger.deleted) != 0 {
		t.Fatal("pair message deleted before the TTL elapsed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(messenger.deleted) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(messenger.deleted) != 1 {
		t.Fatal("pair message was not auto-deleted after the TTL")
	}
	got := messenger.deleted[0]
	if got.peerID != int64(chat.PeerID) || got.messageID != 1 {
		t.Fatalf("auto-deleted %+v, want peerID %d messageID 1", got, chat.PeerID)
	}
}

func TestBroadcastRejectsStaleDailyTimeGroupAndOptIns(t *testing.T) {
	group, other, daily, changedTime := model.GroupName("A"), model.GroupName("B"), "07:30", "08:00"
	queued := &model.Chat{
		ID:               1,
		PeerID:           101,
		GroupName:        &group,
		DailySendingTime: &daily,
		PairSending:      true,
		ChangeAlert:      true,
	}
	cases := []struct {
		name   string
		kind   model.BroadcastKind
		mutate func(*model.Chat)
	}{
		{"daily disabled", model.BDaily, func(chat *model.Chat) { chat.DailySendingTime = nil }},
		{"daily moved", model.BDaily, func(chat *model.Chat) { chat.DailySendingTime = &changedTime }},
		{"daily group changed", model.BDaily, func(chat *model.Chat) { chat.GroupName = &other }},
		{"pair disabled", model.BPair, func(chat *model.Chat) { chat.PairSending = false }},
		{"pair group changed", model.BPair, func(chat *model.Chat) { chat.GroupName = &other }},
		{"changes disabled", model.BChange, func(chat *model.Chat) { chat.ChangeAlert = false }},
		{"changes group changed", model.BChange, func(chat *model.Chat) { chat.GroupName = &other }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service, chats, _ := newBroadcastFixture(t, &broadcastMessengerStub{})
			current := *queued
			test.mutate(&current)
			chats.put(&current)
			resolved, err := service.currentRecipient(context.Background(), queued, test.kind)
			if err != nil || resolved != nil {
				t.Fatalf("stale subscription retained: %+v, %v", resolved, err)
			}
		})
	}
}

func TestBroadcastRecipientLookupFailureDoesNotSend(t *testing.T) {
	messenger := &broadcastMessengerStub{}
	service, chats, logs := newBroadcastFixture(t, messenger)
	chats.lookupError = errors.New("database unavailable")
	if err := service.BroadcastText(
		context.Background(), []*model.Chat{{ID: 1, PeerID: 101}}, "Текст",
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	service.jobs.wait(ctx)
	if ctx.Err() != nil {
		t.Fatal("broadcast did not finish")
	}
	if len(messenger.deliveries) != 0 || len(logs.logs) != 0 || logs.finished != 1 {
		t.Fatal("recipient was sent without confirming subscription")
	}
}
