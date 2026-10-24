import { FormEvent, useEffect, useState } from "react";
import { ArrowLeft, ArrowRight, BarChart3, Eye, EyeOff, Loader2, LockKeyhole, Mail, Phone, UserRound } from "lucide-react";

export type AuthUser = { id: number; username?: string; email?: string; phone?: string; display_name: string; avatar?: string };
type AuthResponse = { token: string; user: AuthUser };
type Contact = "email" | "phone";
type LoginMethod = "password" | "code";

function contactKind(value: string): Contact | null {
  const text = value.trim();
  if (/^1[3-9]\d{9}$/.test(text)) return "phone";
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(text)) return "email";
  return null;
}

export function LoginPage({ apiBase, onSuccess, onBack }: {
  apiBase: string;
  onSuccess: (result: AuthResponse) => void;
  onBack: () => void;
}) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [method, setMethod] = useState<LoginMethod>("password");
  const [registerContact, setRegisterContact] = useState<Contact>("email");
  const [identifier, setIdentifier] = useState("");
  const [accountName, setAccountName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [countdown, setCountdown] = useState(0);
  const [sendingCode, setSendingCode] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const isLocal = (hostname: string) => hostname === "localhost" || hostname === "127.0.0.1";
  const pageIsSecure = window.location.protocol === "https:" || isLocal(window.location.hostname);
  let apiIsSecure = false;
  let localTestServer = false;
  try {
    const endpoint = new URL(apiBase, window.location.href);
    apiIsSecure = endpoint.protocol === "https:" || (endpoint.protocol === "http:" && isLocal(endpoint.hostname));
    localTestServer = import.meta.env.DEV && import.meta.env.VITE_ALLOW_HTTP_TEST_LOGIN === "true"
      && isLocal(window.location.hostname) && endpoint.origin === "http://119.91.58.30";
  } catch { /* Invalid endpoints cannot submit credentials. */ }
  const canUseCredentials = pageIsSecure && apiIsSecure;
  const canTestPassword = pageIsSecure && localTestServer && mode === "login" && method === "password" && /^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$/.test(identifier.trim());
  const canSubmit = canUseCredentials || canTestPassword;
  const currentContact = mode === "register" ? registerContact : contactKind(identifier);
  const contactValue = mode === "register" ? (registerContact === "email" ? email : phone) : identifier.trim();
  const validContact = currentContact === "phone" ? /^1[3-9]\d{9}$/.test(contactValue)
    : currentContact === "email" ? contactKind(contactValue) === "email" : false;

  useEffect(() => {
    if (countdown <= 0) return;
    const timer = window.setTimeout(() => setCountdown(countdown - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [countdown]);

  const switchMode = (next: "login" | "register") => {
    setMode(next);
    setCode("");
    setPassword("");
    setConfirmation("");
    setError("");
    setCountdown(0);
  };

  const sendCode = async () => {
    if (!canUseCredentials || !validContact || !currentContact || sendingCode || countdown > 0) return;
    setSendingCode(true);
    setError("");
    const path = currentContact === "phone" ? "phone/send-code" : "email/send-code";
    const body = currentContact === "phone" ? { phone: contactValue } : { email: contactValue.trim().toLowerCase() };
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/${path}`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
      });
      const result = await response.json() as { error?: string };
      if (!response.ok) throw new Error(result.error || "验证码发送失败");
      setCountdown(60);
    } catch (cause) {
      setError(cause instanceof TypeError ? "无法连接服务，请检查网络后重试" : cause instanceof Error ? cause.message : "验证码发送失败");
    } finally { setSendingCode(false); }
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting || !canSubmit) return;
    if (mode === "register" && password !== confirmation) {
      setError("两次输入的密码不一致");
      return;
    }
    if (mode === "login" && method === "code" && !validContact) {
      setError("验证码登录请填写已绑定的邮箱或手机号");
      return;
    }
    setSubmitting(true);
    setError("");
    let path: string;
    let body: Record<string, string | boolean>;
    if (mode === "register") {
      path = "register";
      body = { username: accountName.trim(), password, code };
      body[registerContact] = contactValue.trim();
    } else if (method === "password") {
      path = "login";
      const value = identifier.trim();
      // username keeps the current server's account-password API compatible.
      body = { identifier: value, username: value, password };
    } else {
      path = currentContact === "phone" ? "phone/login" : "email/login";
      body = currentContact === "phone" ? { phone: contactValue, code, create: false } : { email: contactValue.trim().toLowerCase(), code };
    }
    try {
      const response = await fetch(`${apiBase}/api/v1/auth/${path}`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body)
      });
      const result = await response.json() as AuthResponse & { error?: string };
      if (!response.ok) throw new Error(result.error || "暂时无法完成请求，请稍后重试");
      if (!result.token || !result.user) throw new Error("服务返回了无效的登录结果");
      onSuccess(result);
    } catch (cause) {
      setError(cause instanceof TypeError ? "无法连接服务，请检查网络后重试" : cause instanceof Error ? cause.message : "暂时无法完成请求");
    } finally { setSubmitting(false); }
  };

  return (
    <main className="login-page">
      <div className="login-layout">
        <section className="login-intro" aria-label="慧投介绍">
          <button className="login-brand" type="button" onClick={onBack} aria-label="返回慧投首页">
            <span className="login-brand-mark"><BarChart3 size={21} strokeWidth={1.8} /></span>
            <span>慧投 <small>WiseInvest</small></span>
          </button>
          <div className="login-intro-copy">
            <p className="login-kicker">YOUR RESEARCH SPACE</p>
            <h1>把市场信息，<br />整理成自己的判断。</h1>
            <p>跨市场观察、财报研究与 AI 分析，集中在一个清晰的工作空间。</p>
          </div>
          <div className="login-intro-foot"><span>研究有据</span><i /><span>决策有度</span></div>
        </section>

        <section className="login-content">
          <button type="button" className="login-back" onClick={onBack}><ArrowLeft size={16} /> 返回工作台</button>
          <div className="login-card">
            <p className="login-kicker">WISEINVEST ACCOUNT</p>
            <h2>{mode === "register" ? "创建账户" : "欢迎回来"}</h2>
            <p className="login-description">{mode === "register" ? "选择邮箱或手机号注册，设置同一个账号密码。" : "账号、邮箱或手机号都能登录同一个账户。"}</p>
            {!canUseCredentials && <p className="login-notice">{localTestServer ? "服务器当前使用 HTTP。本机仅开放已有测试账号的密码登录；注册和验证码功能需等待 HTTPS。请勿使用正式密码。" : "当前接口尚未启用 HTTPS；接入正式域名后才能提交账户信息。"}</p>}

            {mode === "register" && <div className="login-methods" role="group" aria-label="注册方式">
              <button type="button" className={registerContact === "email" ? "active" : ""} aria-pressed={registerContact === "email"} onClick={() => { setRegisterContact("email"); setCode(""); setCountdown(0); setError(""); }}>邮箱注册</button>
              <button type="button" className={registerContact === "phone" ? "active" : ""} aria-pressed={registerContact === "phone"} onClick={() => { setRegisterContact("phone"); setCode(""); setCountdown(0); setError(""); }}>手机号注册</button>
            </div>}

            <form onSubmit={submit} className="login-form">
              {mode === "login" ? <>
                <label htmlFor="login-identifier">账号 / 邮箱 / 手机号</label>
                <div className="login-input"><UserRound size={18} /><input id="login-identifier" type="text" autoComplete="username" placeholder="输入账号名、邮箱或手机号" value={identifier} onChange={(event) => { setIdentifier(event.target.value); setCode(""); setCountdown(0); setError(""); }} required maxLength={254} /></div>
                <div className="login-method-inline" role="group" aria-label="验证方式">
                  <button type="button" className={method === "password" ? "active" : ""} aria-pressed={method === "password"} onClick={() => { setMethod("password"); setError(""); }}>密码登录</button>
                  <button type="button" className={method === "code" ? "active" : ""} aria-pressed={method === "code"} onClick={() => { setMethod("code"); setError(""); }}>验证码登录</button>
                </div>
              </> : <>
                <label htmlFor="register-account">账号名</label>
                <div className="login-input"><UserRound size={18} /><input id="register-account" type="text" autoComplete="username" placeholder="3–32 位字母、数字等" value={accountName} onChange={(event) => setAccountName(event.target.value)} required minLength={3} maxLength={32} pattern="[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}" /></div>
                {registerContact === "email" ? <><label htmlFor="register-email">邮箱地址</label><div className="login-input"><Mail size={18} /><input id="register-email" type="email" autoComplete="email" placeholder="name@example.com" value={email} onChange={(event) => { setEmail(event.target.value); setCode(""); setCountdown(0); }} required maxLength={254} /></div></> : <><label htmlFor="register-phone">手机号</label><div className="login-input"><Phone size={18} /><input id="register-phone" type="tel" inputMode="numeric" autoComplete="tel" placeholder="中国大陆手机号" value={phone} onChange={(event) => { setPhone(event.target.value.replace(/\D/g, "").slice(0, 11)); setCode(""); setCountdown(0); }} required pattern="1[3-9][0-9]{9}" /></div></>}
              </>}

              {(mode === "register" || method === "password") && <><label htmlFor="login-password">密码</label><div className="login-input"><LockKeyhole size={18} /><input id="login-password" type={showPassword ? "text" : "password"} autoComplete={mode === "login" ? "current-password" : "new-password"} placeholder={mode === "login" ? "输入密码" : "至少 10 个字符"} value={password} onChange={(event) => setPassword(event.target.value)} required minLength={mode === "register" ? 10 : undefined} maxLength={72} /><button type="button" onClick={() => setShowPassword(!showPassword)} aria-label={showPassword ? "隐藏密码" : "显示密码"}>{showPassword ? <EyeOff size={18} /> : <Eye size={18} />}</button></div></>}
              {mode === "register" && <><label htmlFor="login-confirm">确认密码</label><div className="login-input"><LockKeyhole size={18} /><input id="login-confirm" type={showPassword ? "text" : "password"} autoComplete="new-password" placeholder="再次输入密码" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} required /></div></>}
              {(mode === "register" || method === "code") && <><label htmlFor="login-code">{currentContact === "email" ? "邮箱验证码" : currentContact === "phone" ? "短信验证码" : "验证码"}</label><div className="login-input login-code-input"><LockKeyhole size={18} /><input id="login-code" inputMode="numeric" autoComplete="one-time-code" placeholder="6 位验证码" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} required pattern="[0-9]{6}" /><button type="button" onClick={sendCode} disabled={sendingCode || countdown > 0 || !validContact || !canUseCredentials}>{countdown > 0 ? `${countdown} 秒` : sendingCode ? "发送中" : "获取验证码"}</button></div>{mode === "login" && !currentContact && <p className="login-helper">验证码登录仅支持已绑定的邮箱或手机号。</p>}</>}
              {error && <p className="login-error" role="alert">{error}</p>}
              <button className="login-submit" type="submit" disabled={submitting || !canSubmit || (mode === "login" && method === "code" && !validContact)}>{submitting ? <><Loader2 className="spin" size={18} /> 请稍候</> : <>{mode === "register" ? "创建并登录" : "登录账户"}<ArrowRight size={18} /></>}</button>
            </form>
            <div className="login-switch">{mode === "login" ? "还没有账户？" : "已有账户？"}<button type="button" onClick={() => switchMode(mode === "login" ? "register" : "login")}>{mode === "login" ? "立即注册" : "返回登录"}</button></div>
          </div>
          <p className="login-footnote">账户用于同步你的自选与研究记录。市场分析仅供研究参考。</p>
        </section>
      </div>
    </main>
  );
}
