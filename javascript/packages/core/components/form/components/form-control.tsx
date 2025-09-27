import React from 'react';
import { useStyletron } from 'baseui';
import { FormControl as BaseFormControl } from 'baseui/form-control';

import { Label } from './label/label';

import type { FormControlProps } from './types';

export const FormControl: React.FC<FormControlProps> = ({
  label,
  required,
  description,
  caption,
  error,
  children,
}) => {
  const [css] = useStyletron();

  return (
    <div className={css({ width: '100%' })}>
      <BaseFormControl
        label={label && <Label label={label} required={required} description={description} />}
        caption={caption}
        error={error}
      >
        {children}
      </BaseFormControl>
    </div>
  );
};
