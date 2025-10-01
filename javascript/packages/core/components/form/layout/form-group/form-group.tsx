import React from 'react';
import { useStyletron } from 'baseui';

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
  children,
}) => {
  const [css, theme] = useStyletron();

  const enhancedTitle =
    title && (tooltip ?? endEnhancer) ? (
      <div className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale100 })}>
        {title}
        {tooltip ? <HelpTooltip text={tooltip} /> : null}
        {endEnhancer}
      </div>
    ) : (
      title
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
      overrides={{
        BoxContainer: {
          style: {
            marginBottom: theme.sizing.scale600,
          },
        },
      }}
    >
      {children}
    </Box>
  );
};
