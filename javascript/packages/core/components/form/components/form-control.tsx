import React from 'react';
import { mergeOverrides, useStyletron } from 'baseui';
import { FormControl as BaseFormControl } from 'baseui/form-control';

import { Label } from './label/label';

import type { FormControlProps } from './types';

export const FormControl: React.FC<FormControlProps> = ({
  label,
  required,
  description,
  caption,
  error,
  counter,
  overrides = {},
  children,
}) => {
  const [css] = useStyletron();

  return (
    <div className={css({ width: '100%' })}>
      <BaseFormControl
        label={label && <Label label={label} required={required} description={description} />}
        caption={caption}
        counter={counter}
        error={error}
        overrides={mergeOverrides(overrides, {
          ControlContainer: {
            style: {
              // For form fields, spacing is handled by form layout components. The marginBottom
              // provided by FormControl default interferes with form layout spacing.
              marginBottom: 0,
            },
          },
        })}
      >
        {children}
      </BaseFormControl>
    </div>
  );
};
