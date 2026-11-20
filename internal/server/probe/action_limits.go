package probe

import (
	"fmt"
	"time"
)

// Chat and room creation are cheap for a client to spam and expensive to fan
// out to every peer in a section. These limits are deliberately generous: a
// normal player never reaches them, while a scripted flood is cut off before
// any broadcast is built.
const (
	chatLimitCount  = 8
	chatLimitWindow = 3 * time.Second

	createRoomLimitCount  = 3
	createRoomLimitWindow = 10 * time.Second
)

type actionCounter struct {
	WindowStart time.Time
	Count       int
}

// allowAction is a fixed-window counter keyed by account and action. It is
// intentionally not a token bucket: the goal is to bound burst fan-out, not to
// meter sustained gameplay traffic.
func (server *Server) allowAction(key string, limit int, window time.Duration, now time.Time) bool {
	if key == "" || limit <= 0 {
		return true
	}
	server.actionMu.Lock()
	defer server.actionMu.Unlock()
	if server.actionWindows == nil {
		server.actionWindows = make(map[string]actionCounter)
	}
	counter := server.actionWindows[key]
	if counter.WindowStart.IsZero() || now.Sub(counter.WindowStart) >= window {
		counter = actionCounter{WindowStart: now}
	}
	counter.Count++
	server.actionWindows[key] = counter
	if len(server.actionWindows) > 4096 {
		for entryKey, entry := range server.actionWindows {
			if now.Sub(entry.WindowStart) >= window {
				delete(server.actionWindows, entryKey)
			}
		}
	}
	return counter.Count <= limit
}

func (server *Server) allowChat(uin uint32, now time.Time) bool {
	return server.allowAction(fmt.Sprintf("chat:%d", uin), chatLimitCount, chatLimitWindow, now)
}

func (server *Server) allowCreateRoom(uin uint32, now time.Time) bool {
	return server.allowAction(fmt.Sprintf("create-room:%d", uin), createRoomLimitCount, createRoomLimitWindow, now)
}
