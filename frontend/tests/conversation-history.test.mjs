import assert from "node:assert/strict";
import test from "node:test";
import {
  createConversationDraft,
  readConversationHistory,
  serializeConversationHistory
} from "../src/conversation-history.ts";

const market = { id: "us_stock", welcome: "你好，我是美股研究助手。" };
let sequence = 0;
const draft = () => createConversationDraft(market, () => `id-${++sequence}`);

test("opening and switching agents never saves welcome-only conversations", () => {
  const first = draft();
  const second = createConversationDraft({ id: "crypto", welcome: "欢迎研究加密市场。" }, () => `id-${++sequence}`);
  assert.notEqual(first.id, second.id);
  assert.equal(first.messages[0].content, market.welcome);
  assert.equal(serializeConversationHistory([first, second]), "[]");
  assert.deepEqual(readConversationHistory(JSON.stringify([first, second])), []);
});

test("the first user message makes the draft persistent, even if the request later fails", () => {
  const pending = draft();
  const started = {
    ...pending,
    title: "分析苹果的财报",
    messages: [...pending.messages, { id: "question", role: "user", content: "分析苹果的财报" }]
  };
  assert.deepEqual(readConversationHistory(serializeConversationHistory([started])), [started]);
});

test("old empty history is removed without changing genuine messages or backend sessions", () => {
  const empty = draft();
  const whitespace = { ...draft(), messages: [{ id: "blank", role: "user", content: " \n " }] };
  const saved = {
    ...draft(),
    backendId: 42,
    messages: [
      { id: "question", role: "user", content: "之前保存的问题" },
      { id: "reply", role: "assistant", content: "之前保存的回答", thinking: ["研究数据"] }
    ]
  };
  assert.deepEqual(readConversationHistory(JSON.stringify([empty, saved, whitespace])), [saved]);
});

test("invalid stored data cannot break history loading", () => {
  for (const raw of [null, "{", "null", "{}", JSON.stringify([null, {}, { ...draft(), messages: [{ role: "user", content: null }] }])]) {
    assert.deepEqual(readConversationHistory(raw), []);
  }
});
