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
