import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { accountHistoryKey, readAccountHistory } from "./account-session";
import { readConversationHistory, serializeConversationHistory } from "./conversation-history";
import type { SavedConversation } from "./conversation-history";

export function useAccountHistory(apiBase: string, token: string) {
  const scope = accountHistoryKey(apiBase, token);
  const activeScope = useRef(scope); activeScope.current = scope;
  const [history, setHistory] = useState(() => ({ scope, rows:readConversationHistory(readAccountHistory(localStorage, apiBase, token)) }));
  const conversations = history.scope === scope ? history.rows : [];
  useLayoutEffect(() => { setHistory({ scope, rows:readConversationHistory(readAccountHistory(localStorage, apiBase, token)) }); }, [scope]);
  const setConversations = useCallback((update: SavedConversation[] | ((previous: SavedConversation[]) => SavedConversation[])) => {
    if (activeScope.current !== scope) return;
    setHistory(previous => {
      if (activeScope.current !== scope) return previous;
      const rows = previous.scope === scope ? previous.rows : [];
      return { scope, rows:typeof update === "function" ? update(rows) : update };
    });
  }, [scope]);
  useEffect(() => {
    if (scope && history.scope === scope) localStorage.setItem(scope, serializeConversationHistory(history.rows));
  }, [scope, history]);
  useEffect(() => {
    const sync = (event: StorageEvent) => {
      if (scope && event.key === scope && event.storageArea === localStorage) setHistory({ scope, rows:readConversationHistory(event.newValue) });
    };
    window.addEventListener("storage", sync); return () => window.removeEventListener("storage", sync);
  }, [scope]);
  return { conversations, setConversations, historyScope:scope };
}
