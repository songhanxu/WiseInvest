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

function renderMarkdown(source) {
  const lines = String(source || '').replace(/\r\n?/g, '\n').split('\n');
  const rendered = [];
  let inCode = false;
  lines.forEach((line) => {
    if (/^\s*```/.test(line)) {
      rendered.push(inCode ? '</code></pre>' : '<pre style="white-space:pre-wrap;background:#f1f4ef;padding:8px;border-radius:6px"><code>');
      inCode = !inCode;
    } else if (inCode) {
      rendered.push(`${escapeHTML(line)}\n`);
    } else if (!line.trim()) {
      rendered.push('<br/>');
    } else if (/^#{1,3}\s/.test(line)) {
      rendered.push(`<p style="font-weight:600;margin:8px 0 3px">${inlineMarkdown(line.replace(/^#{1,3}\s+/, ''))}</p>`);
    } else if (/^\s*[-*]\s+/.test(line)) {
      rendered.push(`<p style="margin:3px 0;padding-left:10px">• ${inlineMarkdown(line.replace(/^\s*[-*]\s+/, ''))}</p>`);
    } else if (/^\s*\d+\.\s+/.test(line)) {
      rendered.push(`<p style="margin:3px 0;padding-left:10px">${inlineMarkdown(line.trim())}</p>`);
    } else if (/^>\s?/.test(line)) {
      rendered.push(`<p style="color:#69766a;border-left:2px solid #cbd7c8;padding-left:8px">${inlineMarkdown(line.replace(/^>\s?/, ''))}</p>`);
    } else {
      rendered.push(`<p style="margin:3px 0">${inlineMarkdown(line)}</p>`);
    }
  });
  if (inCode) rendered.push('</code></pre>');
  return rendered.join('');
}

module.exports = { renderMarkdown };
