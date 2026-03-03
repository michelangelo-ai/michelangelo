import React, { useCallback, useMemo } from 'react';
import { OnChangeParams, Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { buildSelectOverrides } from './build-select-overrides';
import { formatSelectedValue } from './format-selected-value';

import type { SelectFieldProps, SelectOption } from './types';

export const SelectField: React.FC<SelectFieldProps> = ({
  name,
  label,
  required,
  validate,
  readOnly,
  disabled,
  description,
  caption,
  placeholder,
  options,
  clearable = true,
  searchable = true,
  multi = false,
  creatable = false,
}) => {
  const { input, meta } = useField<string | string[] | number[] | number>(name, {
    required,
    validate,
  });

  const handleChange = useCallback(
    (params: { value: ReadonlyArray<SelectOption> }) => {
      if (multi) {
        const values = params.value.map((item) => item.id);
        input.onChange(values as string[] | number[]);
      } else {
        const value = params.value.length > 0 ? params.value[0].id : '';
        input.onChange(value);
      }
    },
    [input, multi]
  );

  const value = useMemo<SelectOption[]>(() => {
    return formatSelectedValue(input.value).map<SelectOption>((item) => {
      const selectedFromOption = options?.find((option) => option.id === item);
      if (selectedFromOption) return selectedFromOption;

      return { id: item, label: String(item) }; // Creatable options will fall into this case
    });
  }, [input.value, options]);

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <Select
        id={name}
        value={value}
        options={options}
        onChange={handleChange as (params: OnChangeParams) => unknown}
        onBlur={input.onBlur}
        placeholder={!disabled && !readOnly ? placeholder : ''}
        disabled={disabled}
        clearable={!disabled && !readOnly && clearable}
        searchable={searchable}
        multi={multi}
        overrides={buildSelectOverrides(name, disabled, readOnly)}
        creatable={creatable}
        // Modified getOptionLabel in BaseWeb Select to avoid adding "Create" prefix on user input for
        // creatable dropdowns, as creation typically occurs during form submission. The prefix is misleading.
        getOptionLabel={({ option }) => option.label}
      />
    </FormControl>
  );
};
