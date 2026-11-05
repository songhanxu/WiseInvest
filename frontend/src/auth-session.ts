export function tokenIsActive(token: string, now = Date.now()): boolean {
  if (!token) return false;
  try {
    const encoded = token.split(".")[1];
    const payload = JSON.parse(atob(encoded.replace(/-/g, "+").replace(/_/g, "/"))) as { exp?: number };
    return typeof payload.exp === "number" && payload.exp * 1000 > now;
  } catch { return false; }
}
