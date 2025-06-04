/**
 * @description
 * Escapes underscores present in type enums so that e.g. PIPELINE_MANIFEST_TYPE_INVALID
 * or for kubernetes resource identifier names e.g. metadata.name or metadata.namespace
 * does not get mis-renderer as italic content
 */
export function formatMarkdownText(text: string): string {
  text = normalizeSpaces(text);
  const regex = /([A-Za-z])_([A-Za-z])/g;
  return text.replace(regex, '$1\\_$2');
}

/**
 * @description
 * For some reason (most probably because of some internal regexp) the markdown-to-jsx library "hangs"
 * once the sequence of many spaces is encountered. The UI is getting frozen and the browser is not responding.
 * It looks like the slow-down happens in a logarithmic scale:
 * - 50 consequent spaces - still fast
 * - 70 consequent spaces - already slow
 * - 80 consequent spaces - browser hangs
 *
 * This function does a "dirty" fix, of replacing spaces with "·" for those cases when we have a long empty
 * lines (i.e. in error logs output).
 */
const normalizeSpaces = (str: string): string => {
  return str.replace(/ {50,}/g, (match) => '·'.repeat(match.length));
};
