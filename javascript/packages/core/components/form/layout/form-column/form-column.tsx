import React from 'react';
import { useStyletron } from 'baseui';

import type { FormColumnProps } from './types';

export const FormColumn: React.FC<FormColumnProps> = ({ children }) => {
  const [css] = useStyletron();

  return (
    <div className={css({ boxSizing: 'border-box' })}>
      {children}
    </div>
  );
};
