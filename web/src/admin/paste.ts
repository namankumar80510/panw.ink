// Converts pasted rich text (Word, Google Docs, web pages) to Markdown.
// Only text and a few structural marks survive; HTML is never inserted.

function inline(n: Node): string {
  if (n.nodeType === Node.TEXT_NODE) return (n.textContent ?? '').replace(/\s+/g, ' ');
  if (!(n instanceof Element)) return '';
  const inner = () => Array.from(n.childNodes).map(inline).join('');
  const style = n.getAttribute('style') ?? '';
  switch (n.tagName) {
    case 'STRONG': case 'B': return wrap(inner(), '**');
    case 'EM': case 'I': return wrap(inner(), '*');
    case 'S': case 'DEL': case 'STRIKE': return wrap(inner(), '~~');
    case 'BR': return '  \n';
    case 'A': {
      const href = n.getAttribute('href') ?? '';
      const text = inner();
      return /^(https?:|mailto:)/i.test(href) ? `[${text}](${href})` : text;
    }
    case 'SPAN':
      if (/font-weight:\s*(700|bold)/.test(style)) return wrap(inner(), '**');
      if (/font-style:\s*italic/.test(style)) return wrap(inner(), '*');
      return inner();
    case 'SCRIPT': case 'STYLE': case 'IMG': case 'IFRAME': case 'OBJECT': return '';
    default: return inner();
  }
}

function wrap(s: string, mark: string): string {
  const t = s.trim();
  if (!t) return s;
  const lead = s.slice(0, s.length - s.trimStart().length);
  const trail = s.slice(s.trimEnd().length);
  return `${lead}${mark}${t}${mark}${trail}`;
}

function block(n: Node, out: string[]): void {
  if (!(n instanceof Element)) {
    const t = inline(n).trim();
    if (t) out.push(t);
    return;
  }
  switch (n.tagName) {
    case 'P': case 'DIV': { const t = inline(n).trim(); if (t) out.push(t); else if (n.children.length) Array.from(n.childNodes).forEach((c) => block(c, out)); return; }
    case 'H1': case 'H2': out.push('## ' + inline(n).trim()); return;
    case 'H3': out.push('### ' + inline(n).trim()); return;
    case 'H4': case 'H5': case 'H6': out.push('#### ' + inline(n).trim()); return;
    case 'BLOCKQUOTE': { const inner: string[] = []; Array.from(n.childNodes).forEach((c) => block(c, inner)); out.push(inner.map((l) => '> ' + l).join('\n>\n')); return; }
    case 'UL': case 'OL': {
      let i = 1;
      out.push(Array.from(n.children).filter((c) => c.tagName === 'LI').map((li) => (n.tagName === 'OL' ? `${i++}. ` : '- ') + inline(li).trim()).join('\n'));
      return;
    }
    case 'HR': out.push('---'); return;
    case 'SCRIPT': case 'STYLE': case 'META': case 'TITLE': case 'HEAD': return;
    default: Array.from(n.childNodes).forEach((c) => block(c, out));
  }
}

export function htmlToMarkdown(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html'); // inert: scripts never run
  const out: string[] = [];
  Array.from(doc.body.childNodes).forEach((c) => block(c, out));
  return out.join('\n\n').replace(/\n{3,}/g, '\n\n');
}
