function escapeHTML(value) {
  return String(value || '').replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function inlineMarkdown(source) {
  let text = escapeHTML(source);
  text = text.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, label, url) =>
    `<a href="${url}" style="color:#36573d;text-decoration:underline">${label}</a>`);
  text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code style="background:#f1f4ef">$1</code>')
    .replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
  return text;
}

function renderMarkdown(source, { indentParagraphs = false } = {}) {
  const lines = String(source || '').replace(/\r\n?/g, '\n').split('\n');
  const rendered = [];
  let inCode = false;
  let paragraph = [];
  const flush = () => {
    if (!paragraph.length) return;
    const indent = indentParagraphs ? 'text-indent:2em;' : '';
    rendered.push(`<p style="margin:0 0 12px;${indent}line-height:1.8;word-wrap:break-word">${paragraph.map(inlineMarkdown).join('<br/>')}</p>`);
    paragraph = [];
  };
  lines.forEach((line) => {
    if (/^\s*```/.test(line)) {
      flush();
      rendered.push(inCode ? '</code></pre>' : '<pre style="white-space:pre-wrap;background:#f1f4ef;padding:12px;border-radius:8px;margin:12px 0;word-wrap:break-word"><code>');
      inCode = !inCode;
    } else if (inCode) {
      rendered.push(`${escapeHTML(line)}\n`);
    } else if (!line.trim()) {
      flush();
    } else if (/^#{1,3}\s/.test(line)) {
      flush();
      rendered.push(`<p style="font-weight:600;margin:16px 0 8px;line-height:1.5">${inlineMarkdown(line.replace(/^#{1,3}\s+/, ''))}</p>`);
    } else if (/^\s*[-*]\s+/.test(line)) {
      flush();
      rendered.push(`<p style="margin:0 0 8px;padding-left:12px;line-height:1.8">• ${inlineMarkdown(line.replace(/^\s*[-*]\s+/, ''))}</p>`);
    } else if (/^\s*\d+\.\s+/.test(line)) {
      flush();
      rendered.push(`<p style="margin:0 0 8px;padding-left:12px;line-height:1.8">${inlineMarkdown(line.trim())}</p>`);
    } else if (/^>\s?/.test(line)) {
      flush();
      rendered.push(`<p style="margin:8px 0 12px;color:#69766a;border-left:2px solid #cbd7c8;padding-left:12px;line-height:1.8">${inlineMarkdown(line.replace(/^>\s?/, ''))}</p>`);
    } else {
      paragraph.push(line);
    }
  });
  flush();
  if (inCode) rendered.push('</code></pre>');
  return rendered.join('');
}

module.exports = { renderMarkdown };
