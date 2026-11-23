package gm

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Public account registration. It lives on the same SQLite store as the game
// server, so the two never fight over a second writer process.

const (
	registerWindow      = 10 * time.Minute
	registerMaxAttempts = 5
)

type registerWindowEntry struct {
	Start    time.Time
	Attempts int
}

var registerPage = template.Must(template.New("register").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>QQ堂账号注册</title>
<style>
body{font-family:system-ui,sans-serif;background:#1b2430;color:#e8eef5;display:flex;justify-content:center;padding-top:6vh}
.card{width:360px;background:#243040;border-radius:12px;padding:22px;box-shadow:0 10px 30px #0006}
h1{font-size:18px;margin:0 0 6px}p{font-size:12px;color:#9fb0c3;margin:0 0 14px}
label{display:block;font-size:12px;color:#9fb0c3;margin:10px 0 4px}
input{width:100%;box-sizing:border-box;padding:9px 10px;border-radius:8px;border:1px solid #3a4a5f;background:#1b2430;color:#e8eef5}
button{margin-top:16px;width:100%;padding:10px;border:0;border-radius:8px;background:#f2b23e;color:#1b2430;font-weight:700;cursor:pointer}
.msg{margin-top:12px;font-size:13px}
.ok{color:#6ee7a8}.bad{color:#ff9d9d}
</style></head><body><div class="card">
<h1>QQ堂账号注册</h1>
<p>账号为 5-10 位数字，密码 6-16 位字母数字。</p>
<form method="post" action="/register">
<label>账号</label><input name="uin" inputmode="numeric" autocomplete="off" required>
<label>密码</label><input name="password" type="password" required>
<label>确认密码</label><input name="password2" type="password" required>
<label>昵称（可选）</label><input name="nickname" maxlength="16">
<button type="submit">注册</button>
</form>
<div class="msg {{.Class}}">{{.Message}}</div>
</div></body></html>`))

func (server *Server) handleRegisterPage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet, http.MethodPost)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = registerPage.Execute(writer, map[string]string{"Class": "", "Message": ""})
}

func (server *Server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeMethodNotAllowed(writer, http.MethodPost)
		return
	}
	host, _, _ := net.SplitHostPort(request.RemoteAddr)
	if !server.allowRegistration(host, time.Now()) {
		writeJSON(writer, http.StatusTooManyRequests, map[string]any{"ok": false, "message": "注册过于频繁，请稍后再试"})
		return
	}
	uinText := strings.TrimSpace(request.FormValue("uin"))
	password := request.FormValue("password")
	if request.FormValue("password2") != password {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "两次密码不一致"})
		return
	}
	uin64, err := strconv.ParseUint(uinText, 10, 32)
	if err != nil || uin64 < 10000 {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "账号必须是 5 位以上数字"})
		return
	}
	uin := uint32(uin64)
	if err = validateRegistrationPassword(password); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	exists, err := server.store.HasPassword(ctx, uin)
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]any{"ok": false, "message": "数据库不可用"})
		return
	}
	if exists {
		writeJSON(writer, http.StatusConflict, map[string]any{"ok": false, "message": "该账号已被注册"})
		return
	}
	seed := server.seedProfile(uin)
	if nickname := strings.TrimSpace(request.FormValue("nickname")); nickname != "" {
		seed.Nickname = nickname
	}
	if _, err = server.players.LoadOrCreate(ctx, uin, seed); err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]any{"ok": false, "message": "初始化角色失败"})
		return
	}
	if err = server.store.SetPassword(ctx, uin, password); err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]any{"ok": false, "message": "写入账号失败"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "message": "注册成功，可以直接登录", "uin": uin})
}

func validateRegistrationPassword(password string) error {
	if len(password) < 6 || len(password) > 16 {
		return fmt.Errorf("密码长度必须是 6-16 位")
	}
	for _, r := range password {
		if r < 0x21 || r > 0x7e {
			return fmt.Errorf("密码只能使用字母、数字和英文符号")
		}
	}
	return nil
}

func (server *Server) allowRegistration(host string, now time.Time) bool {
	if host == "" {
		return true
	}
	server.registerMu.Lock()
	defer server.registerMu.Unlock()
	if server.registerWindows == nil {
		server.registerWindows = make(map[string]registerWindowEntry)
	}
	entry := server.registerWindows[host]
	if entry.Start.IsZero() || now.Sub(entry.Start) >= registerWindow {
		entry = registerWindowEntry{Start: now}
	}
	entry.Attempts++
	server.registerWindows[host] = entry
	if len(server.registerWindows) > 4096 {
		for key, value := range server.registerWindows {
			if now.Sub(value.Start) >= registerWindow {
				delete(server.registerWindows, key)
			}
		}
	}
	return entry.Attempts <= registerMaxAttempts
}

func writeRegistrationJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
