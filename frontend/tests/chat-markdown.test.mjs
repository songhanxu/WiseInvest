import assert from "node:assert/strict";
import { after, test } from "node:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { build } from "esbuild";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

const testDir = dirname(fileURLToPath(import.meta.url));
const temporary = await mkdtemp(join(testDir, ".chat-markdown-"));
after(() => rm(temporary, { recursive: true, force: true }));
const result = await build({
  entryPoints: [join(testDir, "../src/ChatMarkdown.tsx")],
  bundle: true,
  packages: "external",
  format: "esm",
  platform: "node",
  jsx: "automatic",
  write: false
});
const compiledPath = join(temporary, "ChatMarkdown.mjs");
await writeFile(compiledPath, result.outputFiles[0].contents);
const { ChatMarkdown } = await import(compiledPath);
const render = (content) => renderToStaticMarkup(createElement(ChatMarkdown, { content }));
const sample = await readFile(join(testDir, "fixtures/research-response.md"), "utf8");

test("research replies render headings, nested lists, tables, links and code", () => {
  const html = render(sample);
  assert.match(html, /<h2>公司研究摘要<\/h2>/);
  assert.match(html, /<strong>核心判断：<\/strong>/);
  assert.match(html, /<ol>[\s\S]*<ul>/);
  assert.match(html, /class="chat-table-scroll"[^>]*tabindex="0"/);
  assert.match(html, /<table>[\s\S]*<thead>[\s\S]*<tbody>/);
  assert.match(html, /text-align:right/);
  assert.match(html, /<blockquote>/);
  assert.match(html, /type="checkbox"[^>]*disabled=""/);
  assert.match(html, /<pre><code class="language-python">/);
  assert.match(html, /href="https:\/\/example.com\/report"[^>]*target="_blank"[^>]*rel="noopener noreferrer"/);
  assert.doesNotMatch(html, /## 公司研究摘要|\*\*核心判断/);
});

test("plain text and single line breaks remain readable", () => {
  const html = render("第一行\n第二行\n\n下一段，金额 $100，价格 < 200。");
  assert.match(html, /第一行<br\/>\n第二行/);
  assert.match(html, /<p>下一段，金额 \$100，价格 &lt; 200。<\/p>/);
});

test("untrusted HTML and executable link protocols are not rendered as active content", () => {
  const html = render('<script>alert(1)</script>\n\n[打开](javascript:alert%281%29)\n\n<img src="x" onerror="alert(1)">');
  assert.doesNotMatch(html, /<script|<img|href="javascript:/);
  assert.match(html, /&lt;script&gt;/);
});

test("partial streamed markdown renders and updates to the complete reply", () => {
  for (const content of ["## 公司研", "**研究", "```python\nprint(1)", "| 观察指标 | 本期 |\n| --- | --- |\n| 收入", sample]) {
    assert.doesNotThrow(() => render(content));
  }
  assert.match(render("**研究完成**"), /<strong>研究完成<\/strong>/);
});
