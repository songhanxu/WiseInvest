// JWT claims only partition the local cache. The backend verifies authentication.
export function accountUserId(token: string): number | null {
  try {
    const payload = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    const value = JSON.parse(atob(payload)).user_id;
    return Number.isSafeInteger(value) && value > 0 ? value : null;
  } catch { return null; }
}

export function accountHistoryKey(apiBase: string, token: string) {
  const id = accountUserId(token);
  return id ? `wiseinvest.conversations.v2:${encodeURIComponent(apiBase.replace(/\/$/, ""))}:${id}` : null;
}

type Storage = Pick<globalThis.Storage, "getItem" | "setItem">;
const legacyKey = "wiseinvest.conversations";
const ownerKey = "wiseinvest.conversations.legacy-owner";

export function readAccountHistory(storage: Storage, apiBase: string, token: string) {
  const key = accountHistoryKey(apiBase, token);
  const legacy = storage.getItem(legacyKey);
  let owner = storage.getItem(ownerKey);
  // Assign legacy data at upgrade time only. A later registration cannot claim it.
  if (!owner && legacy) { owner = key || "unassigned"; storage.setItem(ownerKey, owner); }
  if (!key) return null;
  const current = storage.getItem(key);
  if (current !== null) return current;
  if (legacy && owner === key) { storage.setItem(key, legacy); return legacy; }
  return null;
}
