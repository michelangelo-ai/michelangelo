import React from 'react';
import { DatePicker } from 'baseui/datepicker';
import { Cell, Grid } from 'baseui/layout-grid';
import { isArray, isNil } from 'lodash';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { DATE_FORMAT, type DateFieldProps } from './types';
import { useDateFormatters } from './use-date-formatters';

import type { Theme } from 'baseui';
import type { DatepickerOverrides } from 'baseui/datepicker';

const READ_ONLY_OVERRIDES: DatepickerOverrides = {
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
};

const isEmptyInputValue = (value: unknown): boolean => {
  return isNil(value) || value === '' || (isArray(value) && !value.length);
};

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
  noFutureDate,
  dateFormat = DATE_FORMAT.ISO_DATE_STRING,
}) => {
  const { format, parse } = useDateFormatters(dateFormat);

  // Date format can be epoch seconds string or ISO string
  const { input, meta } = useField<string>(name, {
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
    <Grid gridColumns={1} gridMargins={[0]} gridMaxWidth={0}>
      <Cell>
        <FormControl
          label={label}
          required={required}
          description={description}
          caption={caption}
          error={meta.touched && meta.error ? meta.error : undefined}
        >
          <DatePicker
            id={input.name}
            // Form state is string (epoch/ISO), but format() converts it to Date for the picker
            value={input.value as unknown as Date | null}
            onChange={({ date }) => {
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
            overrides={readOnly ? READ_ONLY_OVERRIDES : undefined}
          />
        </FormControl>
      </Cell>
    </Grid>
  );
};
