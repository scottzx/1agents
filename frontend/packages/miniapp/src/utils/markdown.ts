import { Marked } from 'marked';

const instance = new Marked({
  gfm: true,
  breaks: true,
});

/**
 * Compiles a Markdown string into an HTML string suitable for Taro's RichText.
 */
export function renderMarkdown(content: string): string {
  try {
    return instance.parse(content, { async: false }) as string;
  } catch (err) {
    return content;
  }
}
