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
  labelAddon,
  caption,
  error,
  counter,
  children,
}) => {
  const [css, theme] = useStyletron();

  const labelEndEnhancer =
    counter || labelAddon ? (
      <LabelEndEnhancerContent counter={counter} labelAddon={labelAddon} />
    ) : undefined;

  return (
    <div className={css({ width: '100%' })}>
      <BaseFormControl
        label={label && <Label label={label} required={required} description={description} />}
        labelEndEnhancer={labelEndEnhancer}
        caption={
          caption && (
            <TruncatedText>
              <Markdown>{caption}</Markdown>
            </TruncatedText>
          )
        }
        error={error}
        overrides={{
          ControlContainer: {
            style: {
              // For form fields, spacing is handled by form layout components. The marginBottom
              // provided by FormControl default interferes with form layout spacing.
              marginBottom: 0,
            },
          },
          LabelEndEnhancer: {
            style: {
              display: 'flex',
              alignItems: 'center',
              gap: theme.sizing.scale300,
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

const LabelEndEnhancerContent: React.FC<Pick<FormControlProps, 'counter' | 'labelAddon'>> = ({
  counter,
  labelAddon,
}) => {
  const [css, theme] = useStyletron();

  return (
    <>
      {counter && (
        <span
          className={css({
            ...theme.typography.font100,
            color: theme.colors.contentPrimary,
          })}
        >
          {counter.length}/{counter.maxLength}
        </span>
      )}
      {labelAddon}
    </>
  );
};
