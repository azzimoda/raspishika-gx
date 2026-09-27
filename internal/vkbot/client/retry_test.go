package vkclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/object"
)

// fakePhotoTransport wires a client to an in-test VK API: the upload server
// list, the photo file upload itself, the saved-photo list and the message
// send all resolve against the httptest server (whose /upload handler is
// uploadHandler, defaulting to a successful save) and the mock api.Handler.
// uploadCalls/sendCalls count photo file and messages.send invocations; the
// returned upload URL is the absolute path of the in-test storage server, for
// tests that override api.Handler themselves.
func fakePhotoTransport(t *testing.T, uploadHandler func(http.ResponseWriter, *http.Request)) (*Client, *atomic.Int32, *atomic.Int32, string) {
	t.Helper()
	uploadCalls, sendCalls := &atomic.Int32{}, &atomic.Int32{}

	if uploadHandler == nil {
		uploadHandler = func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"server": 1, "photo": "photo_data", "hash": "hash"}`)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		uploadCalls.Add(1)
		uploadHandler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := New("tok", 42, DefaultVersion)
	if err != nil {
		t.Fatal(err)
	}
	c.vk.Handler = func(method string, sliceParams ...api.Params) (api.Response, error) {
		switch method {
		case "photos.getMessagesUploadServer":
			return api.Response{Response: object.RawMessage(`{"upload_url": "` + srv.URL + `/upload"}`)}, nil
		case "photos.saveMessagesPhoto":
			return api.Response{Response: object.RawMessage(`[{"id": 1, "owner_id": 1, "access_key": "key"}]`)}, nil
		case "messages.send":
			sendCalls.Add(1)
			return api.Response{Response: object.RawMessage(`123`)}, nil
		}
		return api.Response{}, nil
	}
	c.vk.Client = srv.Client()
	return c, uploadCalls, sendCalls, srv.URL + "/upload"
}

func TestSendPhotoRetriesOnTransientUploadError(t *testing.T) {
	// Only the first upload fails, the retry succeeds.
	reqs := &atomic.Int32{}
	c, uploadCalls, sendCalls, _ := fakePhotoTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		if reqs.Add(1) == 1 {
			http.Error(w, "server closed the connection", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"server": 1, "photo": "photo_data", "hash": "hash"}`)
	})

	id, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), "caption", nil)
	if err != nil {
		t.Fatalf("SendPhoto failed after transient error: %v", err)
	}
	if uploadCalls.Load() != 2 {
		t.Fatalf("upload calls = %d, want 2", uploadCalls.Load())
	}
	if sendCalls.Load() != 1 {
		t.Fatalf("messages.send calls = %d, want 1", sendCalls.Load())
	}
	if id != 123 {
		t.Fatalf("message id = %d, want 123", id)
	}
}

func TestSendPhotoRetryExhausted(t *testing.T) {
	c, uploadCalls, sendCalls, _ := fakePhotoTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), "caption", nil)
	if err == nil {
		t.Fatal("SendPhoto succeeded, want error")
	}
	if !strings.Contains(err.Error(), "upload VK photo") {
		t.Fatalf("error = %v, want upload VK photo wrapper", err)
	}
	if uploadCalls.Load() != sendRetryAttempts {
		t.Fatalf("upload calls = %d, want %d", uploadCalls.Load(), sendRetryAttempts)
	}
	if sendCalls.Load() != 0 {
		t.Fatalf("messages.send calls = %d, want 0", sendCalls.Load())
	}
}

func TestSendPhotoNoRetryOnForbidden(t *testing.T) {
	c, uploadCalls, sendCalls, uploadURL := fakePhotoTransport(t, nil)
	// API error 901 (ErrMessagesDenySend) surfaces at the save step: permanent,
	// so a single attempt and no re-upload.
	c.vk.Handler = func(method string, _ ...api.Params) (api.Response, error) {
		switch method {
		case "photos.getMessagesUploadServer":
			return api.Response{Response: object.RawMessage(`{"upload_url": "` + uploadURL + `"}`)}, nil
		case "photos.saveMessagesPhoto":
			return api.Response{}, &api.Error{Code: api.ErrMessagesDenySend}
		}
		return api.Response{}, nil
	}

	_, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), "caption", nil)
	if err == nil {
		t.Fatal("SendPhoto succeeded, want forbidden error")
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != api.ErrMessagesDenySend {
		t.Fatalf("error = %v, want ErrMessagesDenySend", err)
	}
	if uploadCalls.Load() != 1 {
		t.Fatalf("upload calls = %d, want 1 (no retry)", uploadCalls.Load())
	}
	if sendCalls.Load() != 0 {
		t.Fatalf("messages.send calls = %d, want 0", sendCalls.Load())
	}
}

func TestSendPhotoNoRetryOnCancellation(t *testing.T) {
	c, uploadCalls, sendCalls, _ := fakePhotoTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := c.SendPhoto(ctx, 1, "photo.png", []byte("img"), "caption", nil)
	if err == nil {
		t.Fatal("SendPhoto succeeded on cancelled context")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("cancelled context retried instead of returning")
	}
	if uploadCalls.Load() > 1 {
		t.Fatalf("upload calls = %d, want at most 1 on cancellation", uploadCalls.Load())
	}
	if sendCalls.Load() != 0 {
		t.Fatalf("messages.send calls = %d, want 0", sendCalls.Load())
	}
}

func TestSendPhotoSharedRandomIDAcrossSendRetry(t *testing.T) {
	// A transient failure during messages.send must retry with the same
	// random_id so VK deduplicates instead of delivering two messages.
	var sendAttempts atomic.Int32
	ids := make([]string, 0, 2)
	c, _, _, uploadURL := fakePhotoTransport(t, nil)
	c.vk.Handler = func(method string, sliceParams ...api.Params) (api.Response, error) {
		switch method {
		case "photos.getMessagesUploadServer":
			return api.Response{Response: object.RawMessage(`{"upload_url": "` + uploadURL + `"}`)}, nil
		case "photos.saveMessagesPhoto":
			return api.Response{Response: object.RawMessage(`[{"id": 1, "owner_id": 1, "access_key": "key"}]`)}, nil
		case "messages.send":
			id := fmt.Sprint(sliceParams[0]["random_id"])
			ids = append(ids, id)
			if sendAttempts.Add(1) == 1 {
				return api.Response{}, errors.New("EOF")
			}
			return api.Response{Response: object.RawMessage(`123`)}, nil
		}
		return api.Response{}, nil
	}

	id, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), "caption", nil)
	if err != nil {
		t.Fatalf("SendPhoto failed: %v", err)
	}
	if id != 123 {
		t.Fatalf("message id = %d, want 123", id)
	}
	if sendAttempts.Load() != 2 {
		t.Fatalf("messages.send calls = %d, want 2", sendAttempts.Load())
	}
	if len(ids) != 2 || ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("random_id across retries = %v, want equal non-empty", ids)
	}
}

// TestSendPhotoSplitCaptionDistinctRandomIDs covers a caption long enough to be
// split into several messages. VK deduplicates by random_id, so reusing the
// call's id for every part silently drops all but the first one.
func TestSendPhotoSplitCaptionDistinctRandomIDs(t *testing.T) {
	var ids []string
	c, _, _, uploadURL := fakePhotoTransport(t, nil)
	c.vk.Handler = func(method string, sliceParams ...api.Params) (api.Response, error) {
		switch method {
		case "photos.getMessagesUploadServer":
			return api.Response{Response: object.RawMessage(`{"upload_url": "` + uploadURL + `"}`)}, nil
		case "photos.saveMessagesPhoto":
			return api.Response{Response: object.RawMessage(`[{"id": 1, "owner_id": 1, "access_key": "key"}]`)}, nil
		case "messages.send":
			ids = append(ids, fmt.Sprint(sliceParams[0]["random_id"]))
			return api.Response{Response: object.RawMessage(`123`)}, nil
		}
		return api.Response{}, nil
	}

	caption := strings.Repeat("а", 9000)
	if _, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), caption, nil); err != nil {
		t.Fatalf("SendPhoto failed: %v", err)
	}
	parts := splitText(caption)
	if len(parts) < 2 {
		t.Fatalf("test premise broken: caption split into %d part(s), want several", len(parts))
	}
	if len(ids) != len(parts) {
		t.Fatalf("messages.send calls = %d, want one per part (%d)", len(ids), len(parts))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			t.Fatalf("empty random_id in %v", ids)
		}
		if seen[id] {
			t.Fatalf("random_id %q reused across parts %v: VK would drop the duplicates", id, ids)
		}
		seen[id] = true
	}
}

// TestSendPhotoSplitCaptionRetryRepeatsRandomIDs pins the other half of the
// contract: retrying a send that failed on a later part has to repeat the ids
// of the parts VK already accepted, otherwise the retry delivers them twice.
func TestSendPhotoSplitCaptionRetryRepeatsRandomIDs(t *testing.T) {
	var ids []string
	var sendCalls atomic.Int32
	perAttempt := len(splitText(strings.Repeat("а", 9000)))
	c, _, _, uploadURL := fakePhotoTransport(t, nil)
	c.vk.Handler = func(method string, sliceParams ...api.Params) (api.Response, error) {
		switch method {
		case "photos.getMessagesUploadServer":
			return api.Response{Response: object.RawMessage(`{"upload_url": "` + uploadURL + `"}`)}, nil
		case "photos.saveMessagesPhoto":
			return api.Response{Response: object.RawMessage(`[{"id": 1, "owner_id": 1, "access_key": "key"}]`)}, nil
		case "messages.send":
			ids = append(ids, fmt.Sprint(sliceParams[0]["random_id"]))
			// Deliver the first two parts, then fail on the last one so the
			// retry has to reproduce the ids of the parts VK already has.
			if int(sendCalls.Add(1)) == perAttempt {
				return api.Response{}, errors.New("EOF")
			}
			return api.Response{Response: object.RawMessage(`123`)}, nil
		}
		return api.Response{}, nil
	}

	caption := strings.Repeat("а", 9000)
	if _, err := c.SendPhoto(context.Background(), 1, "photo.png", []byte("img"), caption, nil); err != nil {
		t.Fatalf("SendPhoto failed: %v", err)
	}
	if len(ids) != 2*perAttempt {
		t.Fatalf("messages.send calls = %d, want two attempts of %d parts", len(ids), perAttempt)
	}
	for i := 0; i < perAttempt; i++ {
		if ids[i] != ids[perAttempt+i] {
			t.Fatalf("part %d random_id changed on retry: %q then %q (all ids %v)", i, ids[i], ids[perAttempt+i], ids)
		}
	}
}

// TestRandomIDForPartStaysInRange covers the wrap: a base id at the very top of
// the range must not push the derived ids out of what VK accepts.
func TestRandomIDForPartStaysInRange(t *testing.T) {
	for _, base := range []int{1, 2, maxRandomID - 1, maxRandomID} {
		for part := 0; part < 5; part++ {
			got := randomIDForPart(base, part)
			if got < 1 || got > maxRandomID {
				t.Fatalf("randomIDForPart(%d, %d) = %d, want within [1, %d]", base, part, got, maxRandomID)
			}
		}
	}
	if randomIDForPart(maxRandomID, 1) != 1 {
		t.Errorf("randomIDForPart(%d, 1) = %d, want wrap to 1", maxRandomID, randomIDForPart(maxRandomID, 1))
	}
	if randomIDForPart(7, 0) != 7 {
		t.Errorf("randomIDForPart(7, 0) = %d, want 7", randomIDForPart(7, 0))
	}
	if randomIDForPart(7, 1) != 8 {
		t.Errorf("randomIDForPart(7, 1) = %d, want 8", randomIDForPart(7, 1))
	}
}

// TestRetryTransientNoSleepAfterLastAttempt guards the backoff sequence. The
// backoffs are 1s and 2s; sleeping again after the final attempt only delays the
// error the caller is about to see, and it used to log a "retrying" line for a
// retry that never happened.
func TestRetryTransientNoSleepAfterLastAttempt(t *testing.T) {
	calls := 0
	start := time.Now()
	err := retryTransient(context.Background(), func() error {
		calls++
		return errors.New("EOF")
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("retryTransient() = nil, want the last error")
	}
	if calls != sendRetryAttempts {
		t.Fatalf("upload calls = %d, want %d", calls, sendRetryAttempts)
	}
	if want := 3 * time.Second; elapsed > want+500*time.Millisecond {
		t.Fatalf("exhausted retries took %v, want at most %v (1s + 2s of backoff)", elapsed, want)
	}
}

// TestRetryTransientStopsOnNonRetryable covers that a permanent error is not
// retried at all, which is what keeps a fatal VK code from being hammered.
func TestRetryTransientStopsOnNonRetryable(t *testing.T) {
	calls := 0
	err := retryTransient(context.Background(), func() error {
		calls++
		return &api.Error{Code: 901}
	})
	if calls != 1 {
		t.Fatalf("upload calls = %d, want 1 for a non-retryable code", calls)
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != 901 {
		t.Fatalf("retryTransient() = %v, want the original *api.Error", err)
	}
}
