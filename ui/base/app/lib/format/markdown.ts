import MarkdownIt from 'markdown-it';

const md = new MarkdownIt({ html: false, linkify: false });
md.disable('image');
md.renderer.rules.link_open = (tokens, idx, options, _env, self) => {
  const link = tokens[idx]!;
  link.attrSet('class', 'link link-primary');
  link.attrSet('target', '_blank');
  link.attrSet('rel', 'noopener noreferrer');
  return self.renderToken(tokens, idx, options);
};

// renderMarkdown turns markdown into HTML with raw HTML escaped, images off and every link opening in a new tab. SECURITY: html stays false; markdown-it's validateLink drops javascript:, vbscript: and data: links.
export function renderMarkdown(src: string): string {
  return md.render(src);
}
