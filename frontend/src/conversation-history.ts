import type { Market, MarketId } from "./App";

export type Message = {
  id: string;
  role: "assistant" | "user";
  content: string;
  thinking?: string[];
  agentId?: string;
  interrupted?: boolean;
  responseError?: string;
};

export type SavedConversation = {
  id: string;
  marketId: MarketId;
  title: string;
  updatedAt: string;
  messages: Message[];
  backendId?: number;
  kind?: "roundtable";
};

export function hasUserMessage(conversation: SavedConversation): boolean {
  return conversation.messages.some((message) => message.role === "user" && message.content.trim().length > 0);
}

export function createConversationDraft(market: Pick<Market, "id" | "welcome">, createId: () => string): SavedConversation {
  return {
    id: createId(),
    marketId: market.id,
    title: "",
    updatedAt: new Date().toISOString(),
    messages: [{ id: createId(), role: "assistant", content: market.welcome }]
  };
}

export function readConversationHistory(raw: string | null): SavedConversation[] {
  if (!raw) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((item): item is SavedConversation => (
      item != null && typeof item === "object" &&
      typeof item.id === "string" && typeof item.marketId === "string" &&
      typeof item.title === "string" && typeof item.updatedAt === "string" &&
      Array.isArray(item.messages) && item.messages.every((message: unknown) => (
        message != null && typeof message === "object" &&
        "role" in message && (message.role === "user" || message.role === "assistant") &&
        "content" in message && typeof message.content === "string"
      )) && hasUserMessage(item)
    ));
  } catch {
    return [];
  }
}

export function serializeConversationHistory(conversations: SavedConversation[]): string {
  return JSON.stringify(conversations.filter(hasUserMessage));
}
