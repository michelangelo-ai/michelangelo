import React from 'react';
import { useStyletron } from 'baseui';
import { FormControl as BaseFormControl } from 'baseui/form-control';

import { Markdown } from '#core/components/markdown/markdown';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';
import { Label } from './label/label';

import type { FormControlProps } from './types';

export const FormControl: React.FC<FormControlProps> = ({
  label,
  required,
  description,
  caption,
  error,
  counter,
  children,
}) => {
  const [css] = useStyletron();

  return (
    <div className={css({ width: '100%' })}>
      <BaseFormControl
        label={label && <Label label={label} required={required} description={description} />}
        caption={
          caption && (
            <TruncatedText>
              <Markdown>{caption}</Markdown>
            </TruncatedText>
          )
        }
        counter={counter}
        error={error}
        overrides={{
          ControlContainer: {
            style: {
              // For form fields, spacing is handled by form layout components. The marginBottom
              // provided by FormControl default interferes with form layout spacing.
              marginBottom: 0,
            },
          },
          Caption: {
            style: ({ $error, $positive }) => ({
              display: $error || $positive ? 'flex' : 'block',
            }),
          },
        }}
      >
        {children}
      </BaseFormControl>
    </div>
  );
};
