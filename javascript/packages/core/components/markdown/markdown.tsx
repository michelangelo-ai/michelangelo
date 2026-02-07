import MarkdownToJSX from 'markdown-to-jsx';

import { MARKDOWN_OVERRIDES } from './styled-components';
import { formatMarkdownText } from './utils';

import type { FC } from 'react';
import type { Props } from './types';

/**
 * Renders markdown content with custom styled components optimized for the Michelangelo UI.
 *
 * This component parses markdown text and renders it using custom overrides that match
 * the application's theme and design system. It automatically formats the markdown text
 * to ensure proper rendering of code blocks, links, and other markdown elements.
 *
 * Features:
 * - Custom styled components for headings, links, code blocks, lists
 * - Automatic text formatting for improved markdown parsing
 * - BaseUI theme integration
 * - Safe handling of non-string children
 *
 * @param props.children - Markdown string to render. Non-string children are passed through unchanged.
 *
 * @example
 * ```tsx
 * // Basic markdown rendering
 * <Markdown>
 *   # Heading
 *   This is **bold** and *italic* text.
 * </Markdown>
 *
 * // With code blocks
 * <Markdown>
 *   ```javascript
 *   const greeting = "Hello World";
 *   ```
 * </Markdown>
 *
 * // With links
 * <Markdown>
 *   Visit [our docs](https://docs.example.com) for more info.
 * </Markdown>
 *
 * // In help tooltips or descriptions
 * <HelpTooltip text="Use **markdown** for *formatting*" />
 * ```
 */
export const Markdown: FC<Props> = ({ children }) => {
  if (typeof children !== 'string') return <>{children}</>;

  return (
    <MarkdownToJSX options={{ overrides: MARKDOWN_OVERRIDES }}>
      {formatMarkdownText(children)}
    </MarkdownToJSX>
  );
};
