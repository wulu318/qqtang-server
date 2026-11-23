// Package portal serves the public player-facing pages (registration and
// leaderboard) from the game's directory port. It is independent from the GM
// service: disabling GM never removes player registration.
package portal

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"qqtang/internal/protocol/game"
	"qqtang/internal/server/application"
	"qqtang/internal/server/persistence"
)

const (
	registerWindow      = 10 * time.Minute
	registerMaxAttempts = 5
	captchaLifetime     = 10 * time.Minute
)

type SeedProfile func(uint32) game.PlayerProfile

type Server struct {
	store   *persistence.PlayerStore
	players *application.PlayerService
	seed    SeedProfile
	mu      sync.Mutex
	windows map[string]windowEntry
	secret  []byte
	handler http.Handler
}

type windowEntry struct {
	Start    time.Time
	Attempts int
}

func New(store *persistence.PlayerStore, players *application.PlayerService, seed SeedProfile) *Server {
	server := &Server{store: store, players: players, seed: seed, windows: make(map[string]windowEntry)}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets/", server.serveAsset)
	mux.HandleFunc("/leaderboard", server.handleLeaderboard)
	mux.HandleFunc("/register", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			server.handleRegisterPage(writer, request)
			return
		}
		server.handleRegister(writer, request)
	})
	server.handler = securityHeaders(mux)
	return server
}

func (server *Server) Handler() http.Handler { return server.handler }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		if request.Method != http.MethodGet && request.Method != http.MethodHead && !sameOrigin(request) {
			http.Error(writer, "拒绝跨站修改请求", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func sameOrigin(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	return strings.HasSuffix(origin, "://"+request.Host)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = jsonEncode(writer, value)
}

func (server *Server) captchaKey() []byte {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.secret == nil {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			secret = []byte(time.Now().String())
		}
		server.secret = secret
	}
	return server.secret
}

func (server *Server) makeCaptcha(now time.Time) (string, string) {
	a := int(now.UnixNano()%7) + 2
	b := int(now.UnixNano()/7%8) + 1
	expiry := now.Add(captchaLifetime).Unix()
	payload := fmt.Sprintf("%d,%d,%d", a, b, expiry)
	mac := hmac.New(sha256.New, server.captchaKey())
	mac.Write([]byte(payload))
	return fmt.Sprintf("%d + %d = ?", a, b), payload + ":" + hex.EncodeToString(mac.Sum(nil))
}

func (server *Server) verifyCaptcha(token, answer string, now time.Time) bool {
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return false
	}
	fields := strings.Split(parts[0], ",")
	if len(fields) != 3 {
		return false
	}
	a, err1 := strconv.Atoi(fields[0])
	b, err2 := strconv.Atoi(fields[1])
	expiry, err3 := strconv.ParseInt(fields[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || now.Unix() > expiry {
		return false
	}
	mac := hmac.New(sha256.New, server.captchaKey())
	mac.Write([]byte(parts[0]))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(parts[1])) {
		return false
	}
	value, err := strconv.Atoi(strings.TrimSpace(answer))
	return err == nil && value == a+b
}

func (server *Server) allowRegistration(host string, now time.Time) bool {
	if host == "" {
		return true
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	entry := server.windows[host]
	if entry.Start.IsZero() || now.Sub(entry.Start) >= registerWindow {
		entry = windowEntry{Start: now}
	}
	entry.Attempts++
	server.windows[host] = entry
	return entry.Attempts <= registerMaxAttempts
}

func (server *Server) handleRegisterPage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", "GET, POST")
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	question, token := server.makeCaptcha(time.Now())
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = registerPage.Execute(writer, map[string]string{"Class": "", "Message": "", "Question": question, "Token": token})
}

func (server *Server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", "POST")
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	host, _, _ := net.SplitHostPort(request.RemoteAddr)
	if !server.allowRegistration(host, time.Now()) {
		writeJSON(writer, http.StatusTooManyRequests, map[string]any{"ok": false, "message": "注册过于频繁，请稍后再试"})
		return
	}
	if !server.verifyCaptcha(request.FormValue("captcha_token"), request.FormValue("captcha"), time.Now()) {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "验证码不正确或已过期"})
		return
	}
	password := request.FormValue("password")
	if request.FormValue("password2") != password {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "两次密码不一致"})
		return
	}
	uin64, err := strconv.ParseUint(strings.TrimSpace(request.FormValue("uin")), 10, 32)
	if err != nil || uin64 < 10000 {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "账号必须是 5 位以上数字"})
		return
	}
	if err = validatePassword(password); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	uin := uint32(uin64)
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
	seed := server.seed(uin)
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

func validatePassword(password string) error {
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

type leaderboardEntry struct {
	Nickname string `json:"nickname"`
	Level    uint16 `json:"level"`
	Money    uint32 `json:"money"`
	Wins     uint32 `json:"wins"`
	Points   uint32 `json:"points"`
}

func (server *Server) handleLeaderboard(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", "GET")
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	entries, err := server.collect(request.Context())
	if err != nil {
		http.Error(writer, "读取排行榜失败", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = leaderboardPage.Execute(writer, entries)
}

func (server *Server) collect(ctx context.Context) ([]leaderboardEntry, error) {
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
			Nickname: profile.Nickname, Level: profile.GameInfo.Degree,
			Money: profile.GameInfo.Money, Wins: profile.GameInfo.WinNum, Points: profile.GameInfo.Point,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Money != entries[j].Money {
			return entries[i].Money > entries[j].Money
		}
		return entries[i].Wins > entries[j].Wins
	})
	if len(entries) > 50 {
		entries = entries[:50]
	}
	return entries, nil
}
