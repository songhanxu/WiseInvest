import { FormEvent, useEffect, useRef, useState } from "react";
import { Loader2, UserCircle } from "lucide-react";
import { requestJson } from "./App";

type AccountProfile = { id: number; username?: string; display_name?: string; has_password?: boolean };

export function AccountCard({ apiBase, token, onTokenChange }: { apiBase: string; token: string; onTokenChange: (token: string) => void }) {
  const [profile, setProfile] = useState<AccountProfile | null>(null);
  const [profileError, setProfileError] = useState("");
  const [editing, setEditing] = useState(!token);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);

  useEffect(() => {
    setProfile(null); setProfileError(""); setEditing(!token); setPassword(""); setConfirm(""); setError(""); setBusy(false);
    const controller = new AbortController();
    if (token) requestJson<AccountProfile>(apiBase, "/api/v1/auth/me", token, { signal:controller.signal })
      .then(user => { if (!controller.signal.aborted) setProfile(user); })
      .catch(() => { if (!controller.signal.aborted) setProfileError("暂时无法读取账号信息，可重新登录。"); });
    return () => { controller.abort(); request.current?.abort(); request.current = null; };
  }, [apiBase, token]);

  function changeMode(next: "login" | "register") { setMode(next); setPassword(""); setConfirm(""); setError(""); }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (request.current) return;
    if (mode === "register" && password !== confirm) { setError("两次输入的密码不一致"); return; }
    const controller = new AbortController(); request.current = controller;
    setBusy(true); setError("");
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/${mode}`, { method:"POST", signal:controller.signal, headers:{"Content-Type":"application/json"}, body:JSON.stringify({ username:username.trim(), password }) });
      if (response.status === 404) throw new Error("当前后端尚未支持账号密码，请先部署最新后端。");
      const result = await response.json().catch(() => ({})) as { token?: string; user?: AccountProfile; error?: string };
      if (!response.ok || !result.token || !result.user) throw new Error(result.error || (mode === "register" ? "注册失败，请稍后重试" : "登录失败，请稍后重试"));
      if (!controller.signal.aborted) { setPassword(""); setConfirm(""); onTokenChange(result.token); setProfile(result.user); setEditing(false); }
    } catch (err) { if (!controller.signal.aborted) setError(err instanceof Error ? err.message : "无法连接登录服务"); }
    finally { if (request.current === controller) { request.current = null; setBusy(false); } }
  }

  return <section className="settings-card account-card">
    <UserCircle size={22} /><div><h3>账户</h3><p>使用账号密码登录，同步你的自选、提醒和研究记录。</p></div>
    {token && <div className="account-profile"><div><strong>{profile?.username || profile?.display_name || "当前账户"}</strong><span>{profile ? `账户 ID ${profile.id} · 已登录` : "正在读取账户…"}</span></div><div className="account-profile-actions"><button type="button" onClick={() => { setEditing(value => !value); setError(""); }}>切换账号</button><button type="button" onClick={() => onTokenChange("")}>退出登录</button></div>{profileError && <p className="form-note danger" role="alert">{profileError}</p>}</div>}
    {editing && <><div className="account-tabs" role="tablist" aria-label="登录或注册"><button type="button" role="tab" aria-selected={mode === "login"} disabled={busy} onClick={() => changeMode("login")}>登录</button><button type="button" role="tab" aria-selected={mode === "register"} disabled={busy} onClick={() => changeMode("register")}>注册账号</button></div>
      <form className="account-form" onSubmit={event => void submit(event)}>
        <label>账号<input aria-label="登录账号" name="username" autoComplete="username" autoCapitalize="none" spellCheck={false} minLength={3} maxLength={32} pattern="[A-Za-z0-9_][A-Za-z0-9_.\-]{2,31}" title="3–32 位字母、数字、下划线、点或短横线" required value={username} disabled={busy} onChange={event => setUsername(event.target.value)} placeholder="输入账号" /></label>
        <label>密码<input aria-label={mode === "register" ? "注册密码" : "登录密码"} name="password" type="password" autoComplete={mode === "register" ? "new-password" : "current-password"} minLength={6} maxLength={72} required value={password} disabled={busy} onChange={event => setPassword(event.target.value)} placeholder="至少 6 位" /></label>
        {mode === "register" && <label>确认密码<input aria-label="确认密码" name="confirm_password" type="password" autoComplete="new-password" minLength={6} maxLength={72} required value={confirm} disabled={busy} onChange={event => setConfirm(event.target.value)} placeholder="再次输入密码" /></label>}
        {error && <p className="form-note danger" role="alert">{error}</p>}
        <button className="account-submit" type="submit" disabled={busy}>{busy && <Loader2 size={15} className="spin" />}{busy ? (mode === "register" ? "注册中…" : "登录中…") : mode === "register" ? "注册并登录" : "登录"}</button>
      </form></>}
  </section>;
}
