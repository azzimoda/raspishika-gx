package vkclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/object"
)

const testPeer = ChatPeerOffset + 1

// membersPageJSON renders a messages.getConversationMembers response. Passing
// fewer than 200 members tells the scan the list ended.
func membersPageJSON(members ...[3]any) string {
	items := make([]string, 0, len(members))
	for _, m := range members {
		items = append(items, fmt.Sprintf(`{"member_id": %v, "is_admin": %v, "is_owner": %v}`, m[0], m[1], m[2]))
	}
	return fmt.Sprintf(`{"count": %d, "items": [%s]}`, len(members), strings.Join(items, ","))
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New("tok", 42, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestIsAdminCachesVerdict is the reason the cache exists: without it every
// admin-only command walks the whole conversation member list again.
func TestIsAdminCachesVerdict(t *testing.T) {
	c := newTestClient(t)
	var scans atomic.Int32
	c.vk.Handler = func(method string, _ ...api.Params) (api.Response, error) {
		if method != "messages.getConversationMembers" {
			t.Errorf("unexpected method %q", method)
			return api.Response{}, nil
		}
		scans.Add(1)
		return api.Response{Response: object.RawMessage(membersPageJSON([3]any{7, 1, 0}))}, nil
	}

	for i := range 3 {
		admin, err := c.IsAdmin(context.Background(), testPeer, 7)
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		if !admin {
			t.Fatalf("call %d: IsAdmin = false, want true", i+1)
		}
	}
	if got := scans.Load(); got != 1 {
		t.Fatalf("member scans = %d, want 1 (the verdict is cached)", got)
	}

	// A different user is a different key and has to be looked up.
	if _, err := c.IsAdmin(context.Background(), testPeer, 8); err != nil {
		t.Fatalf("other user: %v", err)
	}
	if got := scans.Load(); got != 2 {
		t.Fatalf("member scans after another user = %d, want 2", got)
	}
}

// TestIsAdminCachesNegativeVerdict matters as much as the positive one: the
// member list ends before the user appears, and that answer must be reused too.
func TestIsAdminCachesNegativeVerdict(t *testing.T) {
	c := newTestClient(t)
	var scans atomic.Int32
	c.vk.Handler = func(_ string, _ ...api.Params) (api.Response, error) {
		scans.Add(1)
		return api.Response{Response: object.RawMessage(membersPageJSON([3]any{7, 1, 0}))}, nil
	}

	for range 2 {
		admin, err := c.IsAdmin(context.Background(), testPeer, 999)
		if err != nil {
			t.Fatalf("IsAdmin: %v", err)
		}
		if admin {
			t.Fatal("IsAdmin = true for a user absent from the member list")
		}
	}
	if got := scans.Load(); got != 1 {
		t.Fatalf("member scans = %d, want 1", got)
	}
}

// TestIsAdminExpiredVerdictRescans keeps the cache from becoming permanent.
func TestIsAdminExpiredVerdictRescans(t *testing.T) {
	c := newTestClient(t)
	var scans atomic.Int32
	c.vk.Handler = func(_ string, _ ...api.Params) (api.Response, error) {
		scans.Add(1)
		return api.Response{Response: object.RawMessage(membersPageJSON([3]any{7, 1, 0}))}, nil
	}

	if _, err := c.IsAdmin(context.Background(), testPeer, 7); err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	// Age the entry instead of sleeping for the TTL.
	c.adminMu.Lock()
	key := adminKey{testPeer, 7}
	verdict := c.adminCache[key]
	verdict.expiresAt = time.Now().Add(-time.Second)
	c.adminCache[key] = verdict
	c.adminMu.Unlock()

	if _, err := c.IsAdmin(context.Background(), testPeer, 7); err != nil {
		t.Fatalf("IsAdmin after expiry: %v", err)
	}
	if got := scans.Load(); got != 2 {
		t.Fatalf("member scans = %d, want 2 (the expired verdict must be refetched)", got)
	}
}

// TestIsAdminScanExhausted is an error, not a "not an administrator": a
// conversation with more members than the cap would otherwise refuse every
// administrator past the cap, and callers treat false as a refusal.
func TestIsAdminScanExhausted(t *testing.T) {
	c := newTestClient(t)
	var pages atomic.Int32
	c.vk.Handler = func(_ string, params ...api.Params) (api.Response, error) {
		pages.Add(1)
		// Always a full page of members who are nobody in particular, so the
		// scan keeps paging until the cap.
		members := make([][3]any, 0, 200)
		base := int(params[0]["offset"].(int))
		for i := range 200 {
			members = append(members, [3]any{base + i + 1, 0, 0})
		}
		return api.Response{Response: object.RawMessage(membersPageJSON(members...))}, nil
	}

	admin, err := c.IsAdmin(context.Background(), testPeer, 999999)
	if err == nil {
		t.Fatalf("IsAdmin = (%v, nil), want an exhaustion error", admin)
	}
	if !errors.Is(err, errAdminScanExhausted) {
		t.Fatalf("IsAdmin error = %v, want errAdminScanExhausted", err)
	}
	if admin {
		t.Fatal("IsAdmin = true on an exhausted scan")
	}
	if want := int32(membersLimit / 200); pages.Load() != want {
		t.Fatalf("pages fetched = %d, want %d", pages.Load(), want)
	}
	// The exhausted answer must not be cached as a negative verdict.
	c.adminMu.Lock()
	_, cached := c.adminCache[adminKey{testPeer, 999999}]
	c.adminMu.Unlock()
	if cached {
		t.Fatal("exhausted scan cached a verdict; the user may be an administrator further down the list")
	}
}

// TestIsAdminPagesUntilMemberFound covers the multi-page path and that the
// offset is advanced by the page size.
func TestIsAdminPagesUntilMemberFound(t *testing.T) {
	c := newTestClient(t)
	var offsets []int
	c.vk.Handler = func(_ string, params ...api.Params) (api.Response, error) {
		offset := params[0]["offset"].(int)
		offsets = append(offsets, offset)
		if offset == 0 {
			members := make([][3]any, 0, 200)
			for i := range 200 {
				members = append(members, [3]any{i + 1, 0, 0})
			}
			return api.Response{Response: object.RawMessage(membersPageJSON(members...))}, nil
		}
		return api.Response{Response: object.RawMessage(membersPageJSON([3]any{201, 0, 1}))}, nil
	}

	admin, err := c.IsAdmin(context.Background(), testPeer, 201)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if !admin {
		t.Fatal("IsAdmin = false, want true for the owner on the second page")
	}
	if len(offsets) != 2 || offsets[0] != 0 || offsets[1] != 200 {
		t.Fatalf("offsets = %v, want [0 200]", offsets)
	}
}

// TestIsAdminPrivatePeerNeedsNoAPI keeps the private-conversation shortcut: the
// only admin of a private chat is the other side.
func TestIsAdminPrivatePeerNeedsNoAPI(t *testing.T) {
	c := newTestClient(t)
	c.vk.Handler = func(method string, _ ...api.Params) (api.Response, error) {
		t.Errorf("unexpected API call %q for a private conversation", method)
		return api.Response{}, nil
	}

	if admin, err := c.IsAdmin(context.Background(), 42, 42); err != nil || !admin {
		t.Fatalf("IsAdmin(42, 42) = (%v, %v), want (true, nil)", admin, err)
	}
	if admin, err := c.IsAdmin(context.Background(), 42, 43); err != nil || admin {
		t.Fatalf("IsAdmin(42, 43) = (%v, %v), want (false, nil)", admin, err)
	}
	if admin, err := c.IsAdmin(context.Background(), testPeer, 0); err != nil || admin {
		t.Fatalf("IsAdmin(_, 0) = (%v, %v), want (false, nil)", admin, err)
	}
}

// TestIsAdminPropagatesAPIFailure keeps a failed scan out of the cache.
func TestIsAdminPropagatesAPIFailure(t *testing.T) {
	c := newTestClient(t)
	var scans atomic.Int32
	c.vk.Handler = func(_ string, _ ...api.Params) (api.Response, error) {
		scans.Add(1)
		return api.Response{}, &api.Error{Code: 6}
	}

	if _, err := c.IsAdmin(context.Background(), testPeer, 7); err == nil {
		t.Fatal("IsAdmin = nil error for a failed scan")
	}
	if _, err := c.IsAdmin(context.Background(), testPeer, 7); err == nil {
		t.Fatal("second IsAdmin = nil error")
	}
	if got := scans.Load(); got != 2 {
		t.Fatalf("scans = %d, want 2: a failed scan must not be cached", got)
	}
}
