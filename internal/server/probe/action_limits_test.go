package probe

import (
	"testing"
	"time"
)

func TestAllowActionEnforcesFixedWindow(t *testing.T) {
	var server Server
	now := time.Unix(1_700_000_000, 0)
	for attempt := 1; attempt <= chatLimitCount; attempt++ {
		if !server.allowChat(1_000_001, now) {
			t.Fatalf("chat %d rejected before the limit", attempt)
		}
	}
	if server.allowChat(1_000_001, now) {
		t.Fatal("chat flood was not rejected")
	}
	if !server.allowChat(1_000_002, now) {
		t.Fatal("limit leaked across accounts")
	}
	if !server.allowChat(1_000_001, now.Add(chatLimitWindow)) {
		t.Fatal("window did not reset")
	}
}

func TestCreateRoomLimitIsSeparateFromChat(t *testing.T) {
	var server Server
	now := time.Unix(1_700_000_000, 0)
	for attempt := 1; attempt <= createRoomLimitCount; attempt++ {
		if !server.allowCreateRoom(1_000_003, now) {
			t.Fatalf("room creation %d rejected before the limit", attempt)
		}
	}
	if server.allowCreateRoom(1_000_003, now) {
		t.Fatal("room creation flood was not rejected")
	}
	if !server.allowChat(1_000_003, now) {
		t.Fatal("chat was blocked by the room-creation counter")
	}
}
