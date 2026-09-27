package vkclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// longPollScript drives one response per a_check poll. A nil entry ends the
// session without a response, which is how a broken connection looks to the SDK.
type longPollStep struct {
	ts      string
	updates string
	// broken replies with a body the SDK cannot parse, which kills the session
	// and forces Client.Run to reconnect. Hanging up on the connection instead
	// does not work: Go's transport transparently retries a GET whose connection
	// was closed before any response byte arrived, so the session survived.
	broken bool
}

// fakeVKServe stands in for the VK API and the Bots Long Poll endpoint, so the
// real Client.Run loop can be driven end to end: no SDK internals are stubbed
// out, and reconnect, cursor handling and cancellation are all exercised as the
// production code path.
type fakeVKServe struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	steps  []longPollStep
	polls  []pollRecord // every a_check, in order
	served int          // how many times groups.getLongPollServer answered
	// serverTs is the cursor handed out by a fresh session. Client.Run is
	// expected to ignore it once it has handled events, which is what
	// resumeFrom is for.
	serverTs string
	// closeAfterServe hangs up on every a_check once the script runs out, so a
	// test that only wants one session does not spin reconnecting.
	stop chan struct{}
}

func newFakeVKServe(t *testing.T, serverTs string) *fakeVKServe {
	t.Helper()
	f := &fakeVKServe{t: t, serverTs: serverTs, stop: make(chan struct{})}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeVKServe) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("act") == "a_check" {
		f.servePoll(w, r)
		return
	}
	// The SDK sends the method name as the last path segment, not as a field.
	switch strings.TrimPrefix(r.URL.Path, "/") {
	case "groups.getLongPollServer":
		f.mu.Lock()
		f.served++
		f.mu.Unlock()
		writeJSON(w, `{"response":{"key":"key-1","server":"`+f.srv.URL+`/lp","ts":"`+f.serverTs+`"}}`)
	case "groups.setLongPollSettings":
		writeJSON(w, `{"response":1}`)
	default:
		f.t.Errorf("unexpected VK method %q", r.URL.Path)
		writeJSON(w, `{"response":1}`)
	}
}

// pollRecord is one a_check request: the cursor it carried and the session it
// belonged to, so a test can tell a poll made by a freshly built session from
// one made by the session that is still running.
type pollRecord struct {
	ts      string
	session int
}

func (f *fakeVKServe) servePoll(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	step, idx := longPollStep{}, len(f.polls)
	f.polls = append(f.polls, pollRecord{ts: r.URL.Query().Get("ts"), session: f.served})
	if idx < len(f.steps) {
		step = f.steps[idx]
	}
	f.mu.Unlock()

	if step.broken {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("<html>a proxy ate the response</html>"))
		return
	}

	select {
	case <-f.stop:
		// The test is done with this bot; park the request until the context
		// cancellation in Client.Run closes it.
		<-r.Context().Done()
		return
	default:
	}

	updates := step.updates
	if updates == "" {
		updates = "[]"
	}
	ts := step.ts
	if ts == "" {
		ts = "1"
	}
	writeJSON(w, `{"ts":"`+ts+`","updates":`+updates+`}`)
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// newRunClient wires a Client to the fake VK API.
func (f *fakeVKServe) newRunClient() *Client {
	f.t.Helper()
	c, err := New("token", 42, "")
	if err != nil {
		f.t.Fatal(err)
	}
	c.vk.MethodURL = f.srv.URL + "/"
	return c
}

func (f *fakeVKServe) pollRecords() []pollRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pollRecord(nil), f.polls...)
}

func (f *fakeVKServe) sessions() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.served
}

// messageUpdate is a message_new event body for the given message.
func messageUpdate(peerID, fromID int, convMsgID int) string {
	return `{"type":"message_new","object":{"message":{"peer_id":` + strconv.Itoa(peerID) +
		`,"from_id":` + strconv.Itoa(fromID) +
		`,"conversation_message_id":` + strconv.Itoa(convMsgID) +
		`,"date":1,"text":"привет","out":0}}}`
}

// TestRunDeduplicatesRepeatedEvents is the point of the in-memory event cache:
// VK can replay a batch after a reconnect, and handling a message twice means
// answering the user twice.
func TestRunDeduplicatesRepeatedEvents(t *testing.T) {
	const update = `{"type":"message_new","object":{"message":{"peer_id":100,"from_id":7,"conversation_message_id":5,"date":1,"text":"привет","out":0}}}`
	f := newFakeVKServe(t, "1")
	// The same event twice in one batch, then once more under a new id.
	f.steps = []longPollStep{
		{ts: "2", updates: `[` + update + `,` + update + `,` + messageUpdate(100, 7, 6) + `]`},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var got []Message
	c := f.newRunClient()
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(_ context.Context, m Message) error {
			mu.Lock()
			got = append(got, m)
			mu.Unlock()
			if len(got) == 2 {
				cancel()
			}
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("Run did not return after cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("handled %d messages, want 2 (the duplicate dropped): %+v", len(got), got)
	}
	if got[0].ConversationMessageID != 5 || got[1].ConversationMessageID != 6 {
		t.Fatalf("handled %+v, want conversation ids 5 and 6", got)
	}
}

// TestRunResumesFromLastTsAndReconnects covers the cursor across a reconnect. A
// fresh Long Poll session hands out its own cursor starting at the server's
// value; resuming from an older ts than the last event we handled would replay
// everything the cache has already forgotten, and not resuming at all drops
// messages that arrived while the session was down.
func TestRunResumesFromLastTsAndReconnects(t *testing.T) {
	f := newFakeVKServe(t, "1")
	f.steps = []longPollStep{
		{ts: "77", updates: `[` + messageUpdate(100, 7, 1) + `]`},
		{broken: true},
		{ts: "78", updates: `[` + messageUpdate(100, 7, 2) + `]`},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var got []int
	c := f.newRunClient()
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(_ context.Context, m Message) error {
			mu.Lock()
			got = append(got, m.ConversationMessageID)
			mu.Unlock()
			if len(got) == 2 {
				cancel()
			}
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("Run did not return after cancellation")
	}

	mu.Lock()
	handled := append([]int(nil), got...)
	mu.Unlock()
	if len(handled) != 2 || handled[0] != 1 || handled[1] != 2 {
		t.Fatalf("handled %v, want [1 2] across the reconnect", handled)
	}
	if f.sessions() < 2 {
		t.Fatalf("groups.getLongPollServer called %d times, want a new session after the failure", f.sessions())
	}
	polls := f.pollRecords()
	if len(polls) == 0 {
		t.Fatal("no Long Poll request reached the server")
	}
	if polls[0].ts != "1" {
		t.Fatalf("first poll ts = %q, want the server's 1", polls[0].ts)
	}
	// The first poll of the rebuilt session is the one that has to resume: a
	// fresh server hands out its own cursor from 1, and using that would replay
	// every event since the outage.
	var resumed string
	for _, p := range polls {
		if p.session >= 2 {
			resumed = p.ts
			break
		}
	}
	if resumed == "" {
		t.Fatalf("no poll was made by the rebuilt session: %+v", polls)
	}
	if resumed != "77" {
		t.Fatalf("first poll of the rebuilt session carried ts = %q, want 77, the last event we handled", resumed)
	}
}

// TestRunStopsPromptlyOnCancelledContext keeps shutdown honest: Run has to
// return rather than wait out a Long Poll wait that is still open.
func TestRunStopsPromptlyOnCancelledContext(t *testing.T) {
	f := newFakeVKServe(t, "1")
	f.steps = []longPollStep{{}} // answers immediately, with no events

	ctx, cancel := context.WithCancel(context.Background())
	c := f.newRunClient()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, func(context.Context, Message) error { return nil }) }()

	// Give the loop a moment to get into a poll, then cancel.
	time.Sleep(200 * time.Millisecond)
	started := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(started); elapsed > 5*time.Second {
			t.Fatalf("Run took %v to notice the cancellation", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRunSkipsOutgoingAndEmptyEvents keeps the bot from answering itself, and
// from tripping over an event with no peer: a reply to the bot would otherwise
// loop.
func TestRunSkipsOutgoingAndEmptyEvents(t *testing.T) {
	outgoing := `{"type":"message_new","object":{"message":{"peer_id":100,"from_id":42,"conversation_message_id":1,"date":1,"text":"эхо","out":1}}}`
	noPeer := `{"type":"message_new","object":{"message":{"peer_id":0,"from_id":7,"conversation_message_id":2,"date":1,"text":"привет","out":0}}}`
	noFrom := `{"type":"message_new","object":{"message":{"peer_id":100,"from_id":0,"conversation_message_id":3,"date":1,"text":"привет","out":0}}}`
	f := newFakeVKServe(t, "1")
	f.steps = []longPollStep{{ts: "2", updates: `[` + outgoing + `,` + noPeer + `,` + noFrom + `]`}}

	ctx, cancel := context.WithCancel(context.Background())
	handled := make(chan Message, 4)
	c := f.newRunClient()
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(_ context.Context, m Message) error {
			handled <- m
			return nil
		})
	}()

	select {
	case m := <-handled:
		cancel()
		t.Fatalf("handled an event that should have been skipped: %+v", m)
	case <-time.After(time.Second):
	}
	cancel()
	// Wait for Run to finish so nothing reports after the test ends.
	if err := <-done; err != context.Canceled {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
}

// TestRunKeepsGoingAfterHandlerError keeps one user's failure from taking down
// the loop for everyone else: handler errors are logged, not fatal.
//
// The failing message and the one after it arrive in separate polls on purpose.
// Handled in the same batch they would both be delivered even if the error were
// fatal, so the test would pass for the wrong reason.
func TestRunKeepsGoingAfterHandlerError(t *testing.T) {
	f := newFakeVKServe(t, "1")
	f.steps = []longPollStep{
		{ts: "2", updates: `[` + messageUpdate(100, 7, 1) + `]`},
		{ts: "3", updates: `[` + messageUpdate(100, 8, 2) + `]`},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var seen []int64
	c := f.newRunClient()
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, func(_ context.Context, m Message) error {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, m.FromID)
			if m.FromID == 7 {
				return context.DeadlineExceeded // stands in for any handler failure
			}
			cancel()
			return nil
		})
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("Run did not return after cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("handled %v, want both messages despite the first handler error", seen)
	}
}
