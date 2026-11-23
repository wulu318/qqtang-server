package portal

import (
	"encoding/json"
	"html/template"
	"net/http"
)

func jsonEncode(writer http.ResponseWriter, value any) error {
	return json.NewEncoder(writer).Encode(value)
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

var leaderboardPage = template.Must(template.New("leaderboard").Funcs(template.FuncMap{"inc": func(i int) int { return i + 1 }}).Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>伍陆QQ堂 · 排行榜</title>
<style>
body{font-family:system-ui,sans-serif;background:#141a24;color:#eaf2fb;display:flex;justify-content:center;padding:4vh 12px}
.card{width:640px;background:#1c2634;border-radius:14px;padding:20px;box-shadow:0 18px 50px #0007}
h1{font-size:18px;margin:0 0 14px;color:#f2b23e}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{padding:8px 6px;text-align:left;border-bottom:1px solid #2c3a4d}
th{color:#9fb0c3}td.num{text-align:right}
.empty{color:#9fb0c3;font-size:13px}
</style></head><body><div class="card">
<h1>伍陆QQ堂 · 排行榜</h1>
{{if .}}<table><thead><tr><th>#</th><th>昵称</th><th class="num">等级</th><th class="num">糖币</th><th class="num">胜场</th><th class="num">积分</th></tr></thead><tbody>
{{range $i, $e := .}}<tr><td>{{inc $i}}</td><td>{{$e.Nickname}}</td><td class="num">{{$e.Level}}</td><td class="num">{{$e.Money}}</td><td class="num">{{$e.Wins}}</td><td class="num">{{$e.Points}}</td></tr>{{end}}
</tbody></table>{{else}}<div class="empty">还没有数据</div>{{end}}
</div></body></html>`))

const registerCSS = `:root{--candy:#f2b23e;--pink:#ff7ab8;--mint:#6ee7a8}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;background:radial-gradient(1200px 600px at 70% 10%,#2a3a52 0%,#151b26 55%,#0d1219 100%);color:#eaf2fb;font-family:"Segoe UI",system-ui,sans-serif;display:flex;align-items:center;justify-content:center;overflow:hidden}
#bubbles{position:fixed;inset:0;width:100%;height:100%}
.card{position:relative;z-index:2;width:380px;padding:26px;border-radius:18px;background:rgba(28,38,52,.86);border:1px solid rgba(255,255,255,.08);box-shadow:0 24px 60px rgba(0,0,0,.55);backdrop-filter:blur(10px)}
.brand{display:flex;align-items:baseline;gap:8px;margin-bottom:6px}
.pixel{font-family:Consolas,monospace;font-weight:800;letter-spacing:2px;font-size:26px;color:var(--candy);text-shadow:2px 2px 0 #8a5a10}
.qq{font-size:18px;color:var(--pink);font-weight:700}
.hint{margin:0 0 14px;font-size:12px;color:#9fb0c3}
label{display:block;font-size:12px;color:#9fb0c3;margin:10px 0 4px}
input{width:100%;padding:10px;border-radius:9px;border:1px solid #3a4a5f;background:#121a24;color:#eaf2fb;outline:none}
input:focus{border-color:var(--candy);box-shadow:0 0 0 3px rgba(242,178,62,.18)}
button{margin-top:18px;width:100%;padding:12px;border:0;border-radius:10px;background:linear-gradient(180deg,#ffc85c,#e89a1f);color:#2a1c00;font-weight:800;cursor:pointer;box-shadow:0 6px 0 #8a5a10}
button:active{transform:translateY(4px);box-shadow:0 2px 0 #8a5a10}
.msg{margin-top:14px;font-size:13px;min-height:18px}
.ok{color:var(--mint)}.bad{color:#ff9d9d}`

const registerJS = `(function(){
  const canvas=document.getElementById('bubbles'),ctx=canvas.getContext('2d');
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
      b.x+=b.vx;b.y+=b.vy;
      if(b.y<-0.08){b.y=1.08;b.x=Math.random();}
      const px=(b.x+(mouse.x-.5)*0.04*(b.r/20))*w, py=(b.y+(mouse.y-.5)*0.04*(b.r/20))*h, r=b.r*dpr;
      const g=ctx.createRadialGradient(px-r*.3,py-r*.3,r*.15,px,py,r);
      g.addColorStop(0,b.c+'cc');g.addColorStop(1,b.c+'00');
      ctx.globalAlpha=b.a;ctx.fillStyle=g;ctx.beginPath();ctx.arc(px,py,r,0,7);ctx.fill();
      ctx.globalAlpha=b.a*.5;ctx.strokeStyle=b.c;ctx.lineWidth=1.2*dpr;ctx.stroke();
    }
    ctx.globalAlpha=1;requestAnimationFrame(frame);
  }
  frame();
  const card=document.querySelector('.card');
  addEventListener('mousemove',e=>{
    const rx=(e.clientX/innerWidth-.5), ry=(e.clientY/innerHeight-.5);
    card.style.transform='rotateX('+(-ry*4).toFixed(2)+'deg) rotateY('+(rx*6).toFixed(2)+'deg)';
  });
})();`

func (server *Server) serveAsset(writer http.ResponseWriter, request *http.Request) {
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
