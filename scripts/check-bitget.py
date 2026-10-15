#!/usr/bin/env python3
"""Read-only check using the running backend's environment. Never print secrets or assets."""
import base64
import hashlib
import hmac
import json
import subprocess
import time
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check(settings, path, label):
    timestamp = str(int(time.time() * 1000))
    signature = base64.b64encode(hmac.new(settings["BITGET_API_SECRET"].encode(),
                                        (timestamp + "GET" + path).encode(), hashlib.sha256).digest()).decode()
    request = urllib.request.Request("https://api.bitget.com" + path, headers={
        "ACCESS-KEY": settings["BITGET_API_KEY"],
        "ACCESS-PASSPHRASE": settings["BITGET_PASSPHRASE"],
        "ACCESS-TIMESTAMP": timestamp, "ACCESS-SIGN": signature,
        "Content-Type": "application/json", "Accept": "application/json",
    })
    opener = urllib.request.build_opener(NoRedirect())
    try:
        try:
            response = opener.open(request, timeout=10)
        except urllib.error.HTTPError as response_error:
            response = response_error
        with response:
            status = response.code
            try:
                body = json.loads(response.read(4 << 20))
                code = body.get("code", "") if isinstance(body, dict) else ""
            except (ValueError, TypeError):
                code = ""
            # The provider message and asset payload may contain private data.
            # Only a bounded numeric status code is allowed into diagnostics.
            if not isinstance(code, str) or not code.isascii() or not code.isdigit() or len(code) > 8:
                code = "unavailable"
            ok = status == 200 and code == "00000"
            return {"check": label, "http_status": status, "bitget_code": code, "ok": ok}
    except Exception:
        # Transport errors can contain headers or the supplied passphrase.
        return {"check": label, "ok": False, "error": "connection_failed"}


def main():
    try:
        result = subprocess.run(["docker", "inspect", "--format", "{{json .Config.Env}}", "wiseinvest-api"],
                                capture_output=True, text=True, check=True, timeout=10)
        settings = dict(line.split("=", 1) for line in json.loads(result.stdout) if "=" in line)
    except Exception:
        print("无法读取后端运行环境，请在服务器上用 sudo python3 scripts/check-bitget.py 执行。")
        return 1
    for key in ("BITGET_API_KEY", "BITGET_API_SECRET", "BITGET_PASSPHRASE"):
        if not settings.get(key):
            print(json.dumps({"error": "missing_configuration", "field": key}))
            return 1
    settings["BITGET_API_KEY"] = settings["BITGET_API_KEY"].strip()
    settings["BITGET_API_SECRET"] = settings["BITGET_API_SECRET"].strip()
    account_type = settings.get("BITGET_ACCOUNT_TYPE", "uta").strip().lower() or "uta"
    if account_type not in ("uta", "classic"):
        print("BITGET_ACCOUNT_TYPE 应为 uta 或 classic")
        return 1
    print(json.dumps({"account_type": account_type, "credentials": "present", "mode": "read_only"}))
    assets = "/api/v3/account/assets" if account_type == "uta" else "/api/v2/spot/account/assets"
    checks = [(assets, "assets")]
    if settings.get("BITGET_INCLUDE_FUTURES", "true").lower() not in ("false", "0", "f"):
        position = "/api/v3/position/current-position?category=USDT-FUTURES" if account_type == "uta" else "/api/v2/mix/position/all-position?productType=USDT-FUTURES"
        checks.append((position, "positions"))
    success = True
    for path, label in checks:
        outcome = check(settings, path, label)
        print(json.dumps(outcome))
        success = success and outcome["ok"]
    return 0 if success else 1


if __name__ == "__main__":
    raise SystemExit(main())
