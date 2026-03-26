import React from 'react';
import { useStyletron } from 'baseui';

import type { FormGridProps } from './types';

export const FormGrid: React.FC<FormGridProps> = ({ children }) => {
  const [css, theme] = useStyletron();

  return (
    <div
      className={css({
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        gridColumnGap: theme.sizing.scale800,
      })}
    >
      {children}
    </div>
  );
};
