package gm

import (
	"context"
	"net/http"
	"sort"
	"time"
)

type leaderboardEntry struct {
	UIN      uint32 `json:"uin"`
	Nickname string `json:"nickname"`
	Money    uint32 `json:"money"`
	Level    uint16 `json:"level"`
	Wins     uint32 `json:"wins"`
	Points   uint32 `json:"points"`
}

// handleLeaderboard ranks persisted players. The game client has no recovered
// leaderboard command yet, so this is exposed through the GM HTTP surface and
// can back a web page without inventing protocol fields.
func (server *Server) handleLeaderboard(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	uins, err := server.store.ListUINs(ctx)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "读取账号列表失败")
		return
	}
	entries := make([]leaderboardEntry, 0, len(uins))
	for _, uin := range uins {
		profile, loadErr := server.store.Load(ctx, uin)
		if loadErr != nil {
			continue
		}
		entries = append(entries, leaderboardEntry{
			UIN: uin, Nickname: profile.Nickname,
			Money: profile.GameInfo.Money, Level: profile.GameInfo.Degree,
			Wins: profile.GameInfo.WinNum, Points: profile.GameInfo.Point,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Money != entries[j].Money {
			return entries[i].Money > entries[j].Money
		}
		return entries[i].Wins > entries[j].Wins
	})
	if len(entries) > 100 {
		entries = entries[:100]
	}
	writeJSON(writer, http.StatusOK, map[string]any{"entries": entries})
}
