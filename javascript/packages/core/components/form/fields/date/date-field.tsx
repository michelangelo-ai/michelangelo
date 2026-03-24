import React from 'react';
import { DatePicker } from 'baseui/datepicker';
import { isArray, isNil } from 'lodash';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { type DateFieldProps, DateFormat } from './types';
import { useDateFormatters } from './use-date-formatters';

import type { Theme } from 'baseui';

export const DateField: React.FC<DateFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  placeholder,
  disabled,
  description,
  caption,
  labelAddon,
  noFutureDate,
  dateFormat = DateFormat.ISO_DATE_STRING,
}) => {
  const { format, parse } = useDateFormatters(dateFormat);

  const { input, meta } = useField<string, Date | null>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    // Overrides user-supplied format/parse — DateField requires its own
    // formatters to bridge between date format strings and DatePicker Date objects.
    format,
    parse,
  });

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelAddon={labelAddon}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <DatePicker
        id={input.name}
        value={input.value}
        onChange={({ date }: { date: Date | null }) => {
          // For single date pickers, DatePicker onChange can be invoked with null date.
          // This is particularly problematic when user simply opens and closes date selection
          // without selecting a date. In this case, don't propagate the null value to the form.
          if (isNil(date) && isEmptyInputValue(input.value)) return;
          input.onChange(date);
        }}
        placeholder={!disabled && !readOnly ? (placeholder ?? 'MM/dd/yyyy') : ''}
        formatString="MM/dd/yyyy"
        mask="99/99/9999"
        onOpen={input.onFocus}
        onRangeChange={input.onBlur}
        onClose={input.onBlur}
        maxDate={noFutureDate ? new Date() : undefined}
        disabled={disabled}
        overrides={
          readOnly
            ? {
                Input: {
                  props: {
                    readOnly: true,
                    onFocus: () => undefined,
                    overrides: {
                      InputContainer: {
                        style: ({ $theme }: { $theme: Theme }) => ({
                          backgroundColor: $theme.colors.backgroundPrimary,
                        }),
                      },
                    },
                  },
                },
              }
            : undefined
        }
      />
    </FormControl>
  );
};

function isEmptyInputValue(value: unknown): boolean {
  return isNil(value) || value === '' || (isArray(value) && !value.length);
}
