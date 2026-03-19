import React, { useCallback } from 'react';
import { useStyletron } from 'baseui';
import { Checkbox } from 'baseui/checkbox';

import { DescriptionText } from '#core/components/description-text';
import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { Theme } from 'baseui';
import type { CheckboxFieldProps } from './types';

export const CheckboxField: React.FC<CheckboxFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  disabled,
  description,
  caption,
  format,
  parse,
  options,
}) => {
  const [css, theme] = useStyletron();

  const { input, meta } = useField<string[]>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });

  const { value = [], onChange } = input;

  const toggleOption = useCallback(
    (id: string) => {
      const next = value.includes(id) ? value.filter((v) => v !== id) : [...value, id];
      onChange(next);
    },
    [value, onChange]
  );

  const hasDescriptions = options.some((o) => o.description);

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      {options.length === 0 ? (
        <span>No options available</span>
      ) : (
        <div
          className={css({
            display: 'flex',
            flexDirection: hasDescriptions ? 'column' : 'row',
            flexWrap: hasDescriptions ? undefined : 'wrap',
            gap: theme.sizing.scale600,
          })}
        >
          {options.map((option) => (
            <Checkbox
              key={option.id}
              checked={value.includes(option.id)}
              onChange={() => toggleOption(option.id)}
              disabled={disabled}
              overrides={{
                Label: {
                  style: ({ $theme }: { $theme: Theme }) => ({
                    fontSize: $theme.sizing.scale550,
                  }),
                },
              }}
            >
              {option.label}
              {option.description && <DescriptionText>{option.description}</DescriptionText>}
            </Checkbox>
          ))}
        </div>
      )}
    </FormControl>
  );
};
