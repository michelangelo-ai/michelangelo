import React from 'react';
import { useStyletron } from 'baseui';

import { Markdown } from '#core/components/markdown/markdown';

import type { FormNoteProps } from './types';

export const FormNote: React.FC<FormNoteProps> = ({ content }) => {
  const [css, theme] = useStyletron();

  return (
    <div className={css({ ...theme.typography.ParagraphMedium })}>
      <Markdown>{content}</Markdown>
    </div>
  );
};
