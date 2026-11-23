package gm

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	registerWindow      = 10 * time.Minute
	registerMaxAttempts = 5
	captchaLifetime     = 10 * time.Minute
)

type registerWindowEntry struct {
	Start    time.Time
	Attempts int
}

var registerPage = template.Must(template.New("register").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>伍陆QQ堂 · 注册</title>
<link rel="stylesheet" href="/assets/register.css">
</head><body>
<canvas id="bubbles"></canvas>
<main class="card">
  <div class="brand"><span class="pixel">伍陆</span><span class="qq">QQ堂</span></div>
  <p class="hint">5-10 位数字账号 · 6-16 位字母数字密码</p>
  <form method="post" action="/register" autocomplete="off">
    <label>账号</label>
    <input name="uin" inputmode="numeric" pattern="[0-9]{5,10}" required>
    <label>密码</label>
    <input name="password" type="password" minlength="6" maxlength="16" required>
    <label>确认密码</label>
    <input name="password2" type="password" minlength="6" maxlength="16" required>
    <label>昵称（可选）</label>
    <input name="nickname" maxlength="16">
    <label>验证：{{.Question}}</label>
    <input name="captcha" inputmode="numeric" required>
    <input type="hidden" name="captcha_token" value="{{.Token}}">
    <button type="submit">创建角色</button>
  </form>
  <div class="msg {{.Class}}">{{.Message}}</div>
</main>
<script src="/assets/register.js"></script>
</body></html>`))

const registerCSS = `:root{--candy:#f2b23e;--pink:#ff7ab8;--mint:#6ee7a8;--ink:#141a24}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;background:radial-gradient(1200px 600px at 70% 10%,#2a3a52 0%,#151b26 55%,#0d1219 100%);color:#eaf2fb;font-family:"Segoe UI",system-ui,sans-serif;display:flex;align-items:center;justify-content:center;overflow:hidden}
#bubbles{position:fixed;inset:0;width:100%;height:100%;display:block}
.card{position:relative;z-index:2;width:380px;padding:26px;border-radius:18px;background:rgba(28,38,52,.86);border:1px solid rgba(255,255,255,.08);box-shadow:0 24px 60px rgba(0,0,0,.55),inset 0 1px 0 rgba(255,255,255,.06);backdrop-filter:blur(10px);transform:translateZ(0)}
.brand{display:flex;align-items:baseline;gap:8px;margin-bottom:6px}
.pixel{font-family:Consolas,monospace;font-weight:800;letter-spacing:2px;font-size:26px;color:var(--candy);text-shadow:2px 2px 0 #8a5a10,4px 4px 0 rgba(0,0,0,.35)}
.qq{font-size:18px;color:var(--pink);font-weight:700;letter-spacing:1px}
.hint{margin:0 0 14px;font-size:12px;color:#9fb0c3}
label{display:block;font-size:12px;color:#9fb0c3;margin:10px 0 4px}
input{width:100%;padding:10px;border-radius:9px;border:1px solid #3a4a5f;background:#121a24;color:#eaf2fb;outline:none;transition:border-color .2s,box-shadow .2s}
input:focus{border-color:var(--candy);box-shadow:0 0 0 3px rgba(242,178,62,.18)}
button{margin-top:18px;width:100%;padding:12px;border:0;border-radius:10px;background:linear-gradient(180deg,#ffc85c,#e89a1f);color:#2a1c00;font-weight:800;letter-spacing:1px;cursor:pointer;box-shadow:0 6px 0 #8a5a10,0 10px 20px rgba(0,0,0,.35);transition:transform .08s,box-shadow .08s}
button:active{transform:translateY(4px);box-shadow:0 2px 0 #8a5a10}
.msg{margin-top:14px;font-size:13px;min-height:18px}
.ok{color:var(--mint)}.bad{color:#ff9d9d}`

const registerJS = `(function(){
  const canvas=document.getElementById('bubbles');
  const ctx=canvas.getContext('2d');
  let w,h,dpr=Math.min(devicePixelRatio||1,2);
  function resize(){w=canvas.width=innerWidth*dpr;h=canvas.height=innerHeight*dpr;canvas.style.width=innerWidth+'px';canvas.style.height=innerHeight+'px';}
  resize();addEventListener('resize',resize);
  const mouse={x:.5,y:.5};
  addEventListener('mousemove',e=>{mouse.x=e.clientX/innerWidth;mouse.y=e.clientY/innerHeight;});
  const colors=['#f2b23e','#ff7ab8','#6ee7a8','#7cc4ff'];
  const bubbles=Array.from({length:46},()=>({x:Math.random(),y:Math.random(),r:6+Math.random()*26,vx:(Math.random()-.5)*.0006,vy:-(.0004+Math.random()*.0009),c:colors[(Math.random()*colors.length)|0],a:.08+Math.random()*.18}));
  function frame(){
    ctx.clearRect(0,0,w,h);
    for(const b of bubbles){
      b.x+=b.vx; b.y+=b.vy;
      if(b.y<-0.08){b.y=1.08;b.x=Math.random();}
      if(b.x<-0.08)b.x=1.08; if(b.x>1.08)b.x=-0.08;
      const px=(b.x+(mouse.x-.5)*0.04*(b.r/20))*w;
      const py=(b.y+(mouse.y-.5)*0.04*(b.r/20))*h;
      const r=b.r*dpr;
      const g=ctx.createRadialGradient(px-r*.3,py-r*.3,r*.15,px,py,r);
      g.addColorStop(0,b.c+'cc');g.addColorStop(1,b.c+'00');
      ctx.globalAlpha=b.a;ctx.fillStyle=g;ctx.beginPath();ctx.arc(px,py,r,0,7);ctx.fill();
      ctx.globalAlpha=b.a*.5;ctx.strokeStyle=b.c;ctx.lineWidth=1.2*dpr;ctx.stroke();
    }
    ctx.globalAlpha=1;
    requestAnimationFrame(frame);
  }
  frame();
  const card=document.querySelector('.card');
  addEventListener('mousemove',e=>{
    const rx=(e.clientX/innerWidth-.5), ry=(e.clientY/innerHeight-.5);
    card.style.transform='rotateX('+(-ry*4).toFixed(2)+'deg) rotateY('+(rx*6).toFixed(2)+'deg)';
  });
})();`

func (server *Server) captchaKey() []byte {
	server.registerMu.Lock()
	defer server.registerMu.Unlock()
	if server.captchaSecret == nil {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			secret = []byte(time.Now().String())
		}
		server.captchaSecret = secret
	}
	return server.captchaSecret
}

func (server *Server) makeCaptcha(now time.Time) (question, token string) {
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

func (server *Server) serveRegisterAsset(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/assets/register.css":
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = writer.Write([]byte(registerCSS))
	case "/assets/register.js":
		writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = writer.Write([]byte(registerJS))
	default:
		http.NotFound(writer, request)
	}
}

func (server *Server) handleRegisterPage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet, http.MethodPost)
		return
	}
	question, token := server.makeCaptcha(time.Now())
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = registerPage.Execute(writer, map[string]string{"Class": "", "Message": "", "Question": question, "Token": token})
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
	if !server.verifyCaptcha(request.FormValue("captcha_token"), request.FormValue("captcha"), time.Now()) {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "验证码不正确或已过期"})
		return
	}
	uinText := strings.TrimSpace(request.FormValue("uin"))
	password := request.FormValue("password")
	if request.FormValue("password2") != password {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "两次密码不一致"})
		return
	}
	uin64, err := strconv.ParseUint(uinText, 10, 32)
	if err != nil || uin64 < 10000 || uin64 > 9999999999 {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"ok": false, "message": "账号必须是 5-10 位数字"})
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
