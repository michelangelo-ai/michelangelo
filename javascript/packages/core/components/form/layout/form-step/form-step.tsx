import React from 'react';
import { useStyletron } from 'baseui';
import { HeadingSmall, ParagraphSmall } from 'baseui/typography';

import { Markdown } from '#core/components/markdown/markdown';

import type { FormStepProps } from './types';

export const FormStep: React.FC<FormStepProps> = ({ name, description, children }) => {
  const [css, theme] = useStyletron();

  return (
    <section>
      <div
        className={css({
          display: 'flex',
          alignItems: 'flex-start',
          flexDirection: 'column',
          gap: theme.sizing.scale300,
          marginBottom: theme.sizing.scale600,
        })}
      >
        <HeadingSmall margin={0}>{name}</HeadingSmall>
        {description && (
          <ParagraphSmall margin={0}>
            <Markdown>{description}</Markdown>
          </ParagraphSmall>
        )}
      </div>
      {children}
    </section>
  );
};
