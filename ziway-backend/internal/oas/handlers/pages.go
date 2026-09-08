// pages.go — 页面 HTML 渲染层（OAS-CONSOLE-09 A2）。
// HTML 模板经 go:embed 内嵌于 frontend/；占位符 __P<n>__ 依序由 embedRender 以 strings.Replace 显式注入。
// 占位符次序与拆分前 main.go 中各 PageHTML 的字符串拼接顺序逐一对应，输出字节级一致。
package handlers

import (
	_ "embed"
	"fmt"
	"strings"

	"ziway/backend/pkg/envpolicy"
)

//go:embed frontend/login.html
var loginTmpl string

//go:embed frontend/user_mgmt.html
var userMgmtTmpl string

//go:embed frontend/console_home.html
var consoleHomeTmpl string

//go:embed frontend/overview.html
var overviewTmpl string

//go:embed frontend/approvals.html
var approvalsTmpl string

//go:embed frontend/ownership.html
var ownershipTmpl string

//go:embed frontend/admin_accounts.html
var adminAccountsTmpl string

//go:embed frontend/system_config.html
var systemConfigTmpl string

//go:embed frontend/api_keys.html
var apiKeysTmpl string

//go:embed frontend/federation_nodes.html
var federationNodesTmpl string

//go:embed frontend/oauth_clients.html
var oauthClientsTmpl string

//go:embed frontend/audit_logs.html
var auditLogsTmpl string

//go:embed frontend/role_mgmt.html
var roleMgmtTmpl string

//go:embed frontend/org_mgmt.html
var orgMgmtTmpl string

// embedRender 依序替换模板中的显式占位符（每标记替换一次）。
func embedRender(tmpl string, pairs ...string) string {
	html := tmpl
	for i := 0; i+1 < len(pairs); i += 2 {
		html = strings.Replace(html, pairs[i], pairs[i+1], 1)
	}
	return html
}

func loginPageHTML(redirect string, oasEnv envpolicy.Environment, devTokenEnabled bool, oauthClientID, oauthRedirectURI, oauthResponseType, oauthScope, oauthState string) string {
	quickLoginSection := ""
	if envpolicy.IsQuickLoginEnabled(oasEnv) {
		quickLoginSection = `
		<div style="margin-top:24px;padding-top:20px;border-top:1px solid #e5e7eb">
			<p style="font-size:13px;color:#6b7280;margin-bottom:12px">内测快捷登录</p>
			<div style="display:flex;gap:8px;flex-wrap:wrap">
				<button onclick="quickLogin('SU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">SU 管理员</button>
				<button onclick="quickLogin('AU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">AU 运营</button>
				<button onclick="quickLogin('CU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">CU 客户</button>
				<button onclick="quickLogin('GU')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">GU 访客</button>
				<button onclick="quickLogin('EM')" style="padding:6px 14px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:13px">EM 供给</button>
			</div>
		</div>
		<script>
		async function quickLogin(role){
			const btn=document.getElementById('submitBtn');
			btn.disabled=true;
			try{
				const r=await fetch('/api/v1/auth/quick-login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({role})});
				const d=await r.json();
				if(d.code!==200)throw new Error(d.message||'quick login failed');
				handleToken(d.data);
			}catch(ex){alert(ex.message);btn.disabled=false}
		}
		</script>`
	}
	devTokenSection := ""
	if devTokenEnabled {
		devTokenSection = `
		<div style="margin-top:24px;padding-top:20px;border-top:1px solid #e5e7eb">
			<p style="font-size:13px;color:#6b7280;margin-bottom:12px">🔧 开发临时令牌</p>
			<div style="display:flex;gap:8px;align-items:center;margin-bottom:12px">
				<select id="devTokenRole" style="flex:1;padding:8px 12px;border:1px solid #d1d5db;border-radius:6px;font-size:13px">
					<option value="">按角色生成...</option>
					<option value="SU">SU 管理员</option>
					<option value="AU">AU 运营</option>
					<option value="CU">CU 客户</option>
					<option value="GU">GU 访客</option>
					<option value="EM">EM 供给</option>
					<option value="OFM">OFM 域主</option>
					<option value="OVM">OVM 域运营</option>
					<option value="OGM">OGM 域治理</option>
					<option value="OAM">OAM 权限</option>
				</select>
				<button onclick="genDevToken()" style="padding:8px 16px;border:none;border-radius:6px;background:#059669;color:#fff;cursor:pointer;font-size:13px">生成</button>
			</div>
			<div id="devTokenResult" style="display:none">
				<textarea id="devTokenText" readonly style="width:100%;height:80px;padding:8px;border:1px solid #d1d5db;border-radius:6px;font-size:11px;font-family:monospace;resize:none"></textarea>
				<div style="display:flex;gap:8px;margin-top:8px">
					<button onclick="copyDevToken()" style="flex:1;padding:6px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:12px">📋 复制</button>
					<button onclick="redirectWithDevToken()" id="devTokenRedirectBtn" style="flex:1;padding:6px;border:1px solid #d1d5db;border-radius:6px;background:#f9fafb;cursor:pointer;font-size:12px;display:none">🔗 携带令牌跳转</button>
				</div>
				<p id="devTokenInfo" style="font-size:11px;color:#6b7280;margin-top:8px"></p>
			</div>
		</div>
		<script>
		let devTokenValue='';
		async function genDevToken(){
			const role=document.getElementById('devTokenRole').value;
			if(!role){alert('请选择角色');return}
			try{
				const r=await fetch('/api/v1/auth/dev-token',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({role,expires_minutes:30})});
				const d=await r.json();
				if(d.code!==200)throw new Error(d.message||'dev-token failed');
				devTokenValue=d.data.token;
				document.getElementById('devTokenText').value=devTokenValue;
				document.getElementById('devTokenResult').style.display='block';
				document.getElementById('devTokenInfo').textContent='角色: '+d.data.role+' | 用户: '+d.data.username+' | 过期: '+new Date(d.data.expires_at).toLocaleString();
				const redirect=document.getElementById('form').dataset.redirect;
				if(redirect){
					document.getElementById('devTokenRedirectBtn').style.display='block';
				}
			}catch(ex){alert(ex.message)}
		}
		function copyDevToken(){
			navigator.clipboard.writeText(devTokenValue).then(()=>alert('已复制')).catch(()=>{
				const ta=document.getElementById('devTokenText');
				ta.select();document.execCommand('copy');alert('已复制');
			});
		}
		function redirectWithDevToken(){
			const redirect=document.getElementById('form').dataset.redirect;
			if(redirect){
				const sep=redirect.includes('?')?'&':'?';
				window.location.href=redirect+sep+'token='+devTokenValue;
			}
		}
		</script>`
	}
	redirectAttr := ""
	if redirect != "" {
		redirectAttr = `data-redirect="` + redirect + `"`
	}
	oauthAttrs := ""
	if oauthClientID != "" {
		oauthAttrs = fmt.Sprintf(`data-oauth-client="%s" data-oauth-redirect-uri="%s" data-oauth-state="%s"`,
			oauthClientID, oauthRedirectURI, oauthState)
	}
	return embedRender(loginTmpl, "__P0__", redirectAttr, "__P1__", oauthAttrs, "__P2__", quickLoginSection, "__P3__", devTokenSection)
}

func userMgmtPageHTML() string {

	return userMgmtTmpl
}

func consoleHomePageHTML(username, oasEnv, token string) string {

	return embedRender(consoleHomeTmpl, "__P0__", oasEnv, "__P1__", oasEnv, "__P2__", username, "__P3__", token, "__P4__", token, "__P5__", token, "__P6__", token, "__P7__", token, "__P8__", token, "__P9__", token, "__P10__", token, "__P11__", token, "__P12__", token)
}

func overviewPageHTML(username, oasEnv, token string) string {

	return embedRender(overviewTmpl, "__P0__", oasEnv, "__P1__", oasEnv, "__P2__", username, "__P3__", token, "__P4__", token, "__P5__", token, "__P6__", username, "__P7__", username)
}

func approvalsPageHTML(username, oasEnv, token string) string {
	isOUAU := username == "oas-ou-admin" || username == "oas-au-admin"
	canWrite := "false"
	if isOUAU {
		canWrite = "true"
	}

	return embedRender(approvalsTmpl, "__P0__", oasEnv, "__P1__", token, "__P2__", token, "__P3__", token, "__P4__", token, "__P5__", token, "__P6__", token, "__P7__", func() string {
		if !isOUAU {
			return `<div class="readonly-notice">您以只读身份访问（OAM），仅 OU/AU 管理员可发起/审批操作。</div>`
		}
		return ""
	}(), "__P8__", func() string {
		if isOUAU {
			return `<button class="btn btn-primary" onclick="showCreateModal()">+ 发起审批</button>`
		}
		return ""
	}(), "__P9__", token, "__P10__", canWrite, "__P11__", "`", "__P12__", "`", "__P13__", "`", "__P14__", "`", "__P15__", "`", "__P16__", "`", "__P17__", "`", "__P18__", "`")
}

func ownershipPageHTML(username string) string {

	return embedRender(ownershipTmpl, "__P0__", username)
}

func adminAccountsPageHTML(username string) string {

	return adminAccountsTmpl
}

func systemConfigPageHTML(username string) string {

	return systemConfigTmpl
}

func apiKeysPageHTML(username string) string {

	return apiKeysTmpl
}

func federationNodesPageHTML(username string) string {

	return federationNodesTmpl
}

func oauthClientsPageHTML(username string) string {

	return embedRender(oauthClientsTmpl, "__P0__", username)
}

func auditLogsPageHTML() string {

	return auditLogsTmpl
}

func roleMgmtPageHTML() string {

	return roleMgmtTmpl
}

func orgMgmtPageHTML() string {

	return orgMgmtTmpl
}
