import MarkdownToJSX from 'markdown-to-jsx';

import { MARKDOWN_OVERRIDES } from './styled-components';
import { formatMarkdownText } from './utils';

import type { FC } from 'react';
import type { Props } from './types';

export const Markdown: FC<Props> = ({ children }) => {
  if (typeof children !== 'string') return <>{children}</>;

  return (
    <MarkdownToJSX options={{ overrides: MARKDOWN_OVERRIDES }}>
      {formatMarkdownText(children)}
    </MarkdownToJSX>
  );
};
