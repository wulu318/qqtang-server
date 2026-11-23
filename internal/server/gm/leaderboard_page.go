package gm

import (
	"context"
	"html/template"
	"net/http"
	"sort"
	"time"
)

var leaderboardPage = template.Must(template.New("leaderboard").Funcs(template.FuncMap{"inc": func(i int) int { return i + 1 }}).Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>QQ堂排行榜</title>
<style>
body{font-family:system-ui,sans-serif;background:#1b2430;color:#e8eef5;display:flex;justify-content:center;padding:4vh 12px}
.card{width:640px;background:#243040;border-radius:12px;padding:20px;box-shadow:0 10px 30px #0006}
h1{font-size:18px;margin:0 0 14px}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{padding:8px 6px;text-align:left;border-bottom:1px solid #2f3d4f}
th{color:#9fb0c3;font-weight:600}
td.num{text-align:right;font-variant-numeric:tabular-nums}
.empty{color:#9fb0c3;font-size:13px}
</style></head><body><div class="card">
<h1>QQ堂排行榜</h1>
{{if .}}<table><thead><tr><th>#</th><th>昵称</th><th class="num">等级</th><th class="num">糖币</th><th class="num">胜场</th><th class="num">积分</th></tr></thead><tbody>
{{range $i, $e := .}}<tr><td>{{inc $i}}</td><td>{{$e.Nickname}}</td><td class="num">{{$e.Level}}</td><td class="num">{{$e.Money}}</td><td class="num">{{$e.Wins}}</td><td class="num">{{$e.Points}}</td></tr>{{end}}
</tbody></table>{{else}}<div class="empty">还没有数据</div>{{end}}
</div></body></html>`))

func (server *Server) handleLeaderboardPage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet)
		return
	}
	entries, err := server.collectLeaderboard(request.Context(), 50)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "读取排行榜失败")
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = leaderboardPage.Execute(writer, entries)
}

func (server *Server) collectLeaderboard(ctx context.Context, limit int) ([]leaderboardEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	uins, err := server.store.ListUINs(ctx)
	if err != nil {
		return nil, err
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
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
