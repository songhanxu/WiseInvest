import { readFile } from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import ts from 'typescript';

const source = await readFile(new URL('../src/research.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
const { roundtableParticipants, roundtableMentionAt, insertRoundtableMention, readResearchStream, technicalOverview } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

function response(chunks) {
  return new Response(new ReadableStream({ start(controller) { for (const chunk of chunks) controller.enqueue(chunk); controller.close(); } }));
}
const encode = text => new TextEncoder().encode(text);

test('mentions select the exact agents and only all overrides selected roles', () => {
  assert.deepEqual(roundtableParticipants('讨论风险', ['value']), ['value']);
  assert.deepEqual(roundtableParticipants('@量化派 @趋势派', ['value']), ['trend', 'quant']);
  assert.deepEqual(roundtableParticipants('@主持人 @价值派', ['value']), ['orchestrator', 'value']);
  assert.deepEqual(roundtableParticipants('@主持人', ['value']), ['orchestrator']);
  assert.deepEqual(roundtableParticipants('@all 讨论', ['value']), []);
  assert.deepEqual(roundtableParticipants('问题', ['unknown']), []);
});

test('mention suggestions follow the caret and insertion preserves surrounding Chinese text', () => {
  assert.equal(roundtableMentionAt('请@', 2).options.length, 5);
  const text = '请@价分析风险';
  const range = roundtableMentionAt(text, 3);
  assert.deepEqual(range.options.map(agent => agent.id), ['value']);
  const result = insertRoundtableMention(text, range, '价值派');
  assert.equal(result.text, '请@价值派 分析风险');
  assert.equal(result.caret, 6);
  assert.equal(roundtableMentionAt('name@example.com', 16), null);
  assert.equal(roundtableMentionAt('@不存在', 4), null);
  assert.equal(roundtableMentionAt('@价值派 ', 5), null);
});

test('an individual agent error does not discard a later successful reply', async () => {
  const chunks = [];
  await readResearchStream(response([encode('data: {"type":"agent_error","agent_id":"value","content":"AI 服务限流"}\n\ndata: {"type":"content","agent_id":"trend","content":"趋势观点"}\n\ndata: [DONE]\n\n')]), chunk => chunks.push(chunk));
  assert.equal(chunks[0].type, 'agent_error');
  assert.equal(chunks[1].content, '趋势观点');
});

test('SSE accepts single-byte UTF-8 chunks, heartbeats, CRLF and un-terminated DONE', async () => {
  const wire = encode(': heartbeat\r\n\r\ndata:{"type":"agent_start","agent_id":"value"}\r\n\r\ndata: {"type":"content","agent_id":"value","content":"价值观点"}\r\n\r\ndata: [DONE]');
  const received = [];
  await readResearchStream(response([...wire].map(byte => new Uint8Array([byte]))), chunk => received.push(chunk));
  assert.equal(received.length, 2);
  assert.equal(received[1].content, '价值观点');
  assert.equal(received[1].agent_id, 'value');
});

test('server errors and an interrupted stream cannot be reported as a successful discussion', async () => {
  await assert.rejects(readResearchStream(response([encode('data: {"error":"服务不可用"}\n\n')]), () => {}), /服务不可用/);
  const partial = [];
  await assert.rejects(readResearchStream(response([encode('data: {"type":"content","content":"已有回复"}\n\n')]), chunk => partial.push(chunk)), /连接已中断/);
  assert.equal(partial[0].content, '已有回复');
  await assert.rejects(readResearchStream(new Response('{"error":"请先登录"}', { status: 401 }), () => {}), /请先登录/);
  await assert.rejects(readResearchStream(response([encode('data: not-json\n\n')]), () => {}), /回复格式异常/);
});

test('roundtable history retains backend session, role identity and partial replies', async () => {
  const historySource = await readFile(new URL('../src/conversation-history.ts', import.meta.url), 'utf8');
  const compiled = ts.transpileModule(historySource, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022 } });
  const { readConversationHistory, serializeConversationHistory } = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputText).toString('base64')}`);
  const conversation = { id:'round-test', kind:'roundtable', marketId:'a_share', title:'测试讨论', updatedAt:'2026-10-17T00:00:00Z', backendId:42, messages:[{id:'u',role:'user',content:'问题'},{id:'a',role:'assistant',agentId:'quant',content:'部分回复',interrupted:true}] };
  const restored = readConversationHistory(serializeConversationHistory([conversation]));
  assert.deepEqual(restored, [conversation]);
  assert.deepEqual(readConversationHistory('not-json'), []);
  assert.equal(readConversationHistory(serializeConversationHistory([{...conversation,messages:[]}])).length, 0);
});

test('volume baseline excludes latest candle and missing data does not fabricate signals', () => {
  const candles = Array.from({ length: 21 }, (_, index) => ({ date: String(index), open: index + 1, high: index + 1, low: index + 1, close: index + 1, volume: index === 20 ? 200 : 100 }));
  const overview = technicalOverview(candles);
  assert.equal(overview.ma5, 19);
  assert.equal(overview.ma20, 11.5);
  assert.equal(overview.volumeRatio, 2);
  assert.equal(overview.trend, '均线偏强');
  assert.equal(technicalOverview(candles.slice(0, 10)).ma20, null);
  assert.equal(technicalOverview(candles.slice(0, 20)).volumeRatio, null);
  assert.equal(technicalOverview([]).trend, '数据不足');
  assert.equal(technicalOverview(candles.map(row => ({ ...row, volume: 0 }))).volumeRatio, null);
});
