package gm

import (
	"context"
	"html/template"
	"net/http"
	"time"
)

var invitePage = template.Must(template.New("invites").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>邀请码管理</title>
<style>
body{font-family:system-ui,sans-serif;background:#141a24;color:#eaf2fb;padding:24px}
h1{font-size:18px;color:#f2b23e}
button{padding:8px 14px;border:0;border-radius:8px;background:#f2b23e;color:#2a1c00;font-weight:700;cursor:pointer}
table{margin-top:16px;border-collapse:collapse;font-size:13px}
th,td{padding:6px 10px;border-bottom:1px solid #2c3a4d;text-align:left}
.used{color:#ff9d9d}.free{color:#6ee7a8}
</style></head><body>
<h1>邀请码管理</h1>
<p><button onclick="gen(1)">生成 1 个</button> <button onclick="gen(10)">生成 10 个</button></p>
<table id="list"><thead><tr><th>邀请码</th><th>状态</th><th>使用账号</th><th>创建时间</th></tr></thead><tbody></tbody></table>
<script>
async function load(){
  const r=await fetch('/gm/api/invites');const d=await r.json();
  const body=document.querySelector('#list tbody');body.innerHTML='';
  for(const c of d.codes||[]){
    const tr=document.createElement('tr');
    const used=c.used_by>0;
    tr.innerHTML='<td>'+c.code+'</td><td class="'+(used?'used':'free')+'">'+(used?'已使用':'可用')+'</td><td>'+(c.used_by||'')+'</td><td>'+(c.created_utc||'')+'</td>';
    body.appendChild(tr);
  }
}
async function gen(n){
  await fetch('/gm/api/invites',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({count:n})});
  load();
}
load();
</script></body></html>`))

func (server *Server) handleInvitePage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMethodNotAllowed(writer, http.MethodGet)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = invitePage.Execute(writer, nil)
}

func (server *Server) handleInvites(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()
	switch request.Method {
	case http.MethodGet:
		codes, err := server.store.ListInviteCodes(ctx, 200)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, "读取邀请码失败")
			return
		}
		type view struct {
			Code       string `json:"code"`
			CreatedUTC string `json:"created_utc"`
			UsedBy     uint32 `json:"used_by"`
			UsedUTC    string `json:"used_utc"`
		}
		out := make([]view, 0, len(codes))
		for _, code := range codes {
			out = append(out, view{Code: code.Code, CreatedUTC: code.CreatedUTC, UsedBy: code.UsedBy, UsedUTC: code.UsedUTC})
		}
		writeJSON(writer, http.StatusOK, map[string]any{"codes": out})
	case http.MethodPost:
		var input struct {
			Count int `json:"count"`
		}
		if !decodeJSONBody(writer, request, &input) {
			return
		}
		if input.Count <= 0 {
			input.Count = 1
		}
		codes, err := server.store.CreateInviteCodes(ctx, input.Count)
		if err != nil {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "codes": codes})
	default:
		writeMethodNotAllowed(writer, http.MethodGet, http.MethodPost)
	}
}
