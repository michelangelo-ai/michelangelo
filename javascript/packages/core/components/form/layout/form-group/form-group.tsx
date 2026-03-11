import React from 'react';
import { mergeOverrides, useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { CollapsibleBox } from '#core/components/box/collapsible-box';
import { HelpTooltip } from '#core/components/help-tooltip';
import { Markdown } from '#core/components/markdown/markdown';

import type { FormGroupProps } from './types';

export const FormGroup: React.FC<FormGroupProps> = ({
  title,
  description,
  tooltip,
  collapsible = false,
  endEnhancer,
  overrides = {},
  children,
}) => {
  const [css, theme] = useStyletron();

  const titleWithTooltip =
    title && tooltip ? (
      <div className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale100 })}>
        {title}
        <HelpTooltip text={tooltip} />
      </div>
    ) : (
      title
    );

  const enhancedTitle =
    titleWithTooltip && endEnhancer ? (
      <div
        className={css({
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
        })}
      >
        {titleWithTooltip}
        {endEnhancer}
      </div>
    ) : (
      titleWithTooltip
    );

  const enhancedDescription = description ? <Markdown>{description}</Markdown> : undefined;

  if (collapsible) {
    return (
      <CollapsibleBox
        title={enhancedTitle}
        description={enhancedDescription}
        defaultExpanded={false}
        overrides={{
          Content: {
            style: {
              paddingTop: 0,
            },
          },
        }}
      >
        {children}
      </CollapsibleBox>
    );
  }

  return (
    <Box
      title={enhancedTitle}
      description={enhancedDescription}
      overrides={mergeOverrides(
        {
          BoxContainer: {
            style: {
              marginBottom: theme.sizing.scale600,
            },
          },
        },
        overrides
      )}
    >
      {children}
    </Box>
  );
};
