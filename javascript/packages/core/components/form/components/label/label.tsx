import React from 'react';
import { useStyletron } from 'baseui';

import { HelpTooltip } from '#core/components/help-tooltip';

import type { LabelProps } from './types';

export const Label: React.FC<LabelProps> = ({ label, required, description }) => {
  const [css, theme] = useStyletron();

  return (
    <span
      className={css({
        display: 'flex',
        gap: theme.sizing.scale200,
        // prevents the field from form-row mis-alignment when the {field.name} is empty
        minHeight: theme.sizing.scale600,
        alignItems: 'center',
      })}
    >
      {label}
      {required ? <span className={css({ color: theme.colors.negative })}>*</span> : null}
      {description ? <HelpTooltip text={description} /> : null}
    </span>
  );
};
