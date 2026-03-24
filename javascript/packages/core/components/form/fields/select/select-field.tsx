import { useEffect, useMemo } from 'react';
import { filterOptions, OnChangeParams, Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { buildSelectOverrides } from './build-select-overrides';
import { formatSelectedValue } from './format-selected-value';
import { serializeKey } from './serialize-key';

import type { SelectFieldProps, SelectOption } from './types';

export function SelectField<V = string | number>({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  disabled,
  description,
  caption,
  labelAddon,
  placeholder,
  format,
  parse,
  options,
  visibleOptionLimit,
  isLoading = false,
  clearable = true,
  searchable = true,
  multi = false,
  creatable = false,
}: SelectFieldProps<V>) {
  const { input, meta } = useField<V | V[]>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });

  const { baseUIOptions, findByValue, findByKey } = useMemo(() => {
    const map = new Map<string, SelectOption<V>>();
    const adapted = options.map((opt) => {
      const key = serializeKey(opt.id);
      map.set(key, opt);
      return { id: key, label: opt.label, disabled: opt.disabled };
    });
    return {
      baseUIOptions: adapted,
      findByValue: (v: V) => map.get(serializeKey(v)),
      findByKey: (key: string) => map.get(key),
    };
  }, [options]);

  // Clear field value when it doesn't match any available option.
  // Deps intentionally exclude input/multi to avoid re-running on every value change,
  // which would loop since we call onChange inside.
  useEffect(() => {
    if (isLoading || creatable) return;

    const currentValue = input.value;
    if (!currentValue || (Array.isArray(currentValue) && currentValue.length === 0)) return;

    if (multi) {
      const values = currentValue as V[];
      const validValues = values.filter((v) => findByValue(v));
      if (validValues.length !== values.length) {
        input.onChange(validValues);
      }
    } else if (!findByValue(currentValue as V)) {
      input.onChange('' as V | V[]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [findByValue, isLoading]);

  const handleChange = (params: OnChangeParams) => {
    const selected = params.value as Array<{ id: string }>;

    if (multi) {
      input.onChange(selected.map((item) => findByKey(item.id)?.id ?? (item.id as V)));
    } else if (selected.length > 0) {
      input.onChange(findByKey(selected[0].id)?.id ?? (selected[0].id as V));
    } else {
      input.onChange('' as V | V[]);
    }
  };

  const baseUIValue = useMemo(() => {
    const items: Array<{ id: string; label: string; disabled?: boolean }> = [];
    for (const item of formatSelectedValue(input.value)) {
      const key = serializeKey(item);
      const matched = findByKey(key);
      if (matched) {
        items.push({ id: key, label: matched.label, disabled: matched.disabled });
      } else if (creatable) {
        items.push({ id: key, label: String(item) });
      }
    }
    return items;
  }, [input.value, findByKey, creatable]);

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelAddon={labelAddon}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <Select
        id={name}
        value={baseUIValue}
        options={baseUIOptions}
        onChange={handleChange}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        placeholder={!disabled && !readOnly ? placeholder : ''}
        disabled={disabled}
        clearable={!disabled && !readOnly && clearable}
        searchable={searchable}
        multi={multi}
        overrides={buildSelectOverrides(name, disabled, readOnly)}
        creatable={creatable}
        filterOptions={(options, filterValue, excludeOptions, newProps) =>
          filterOptions(options, filterValue, excludeOptions, newProps).slice(0, visibleOptionLimit)
        }
        isLoading={isLoading}
        // Modified getOptionLabel in BaseWeb Select to avoid adding "Create" prefix on user input for
        // creatable dropdowns, as creation typically occurs during form submission. The prefix is misleading.
        getOptionLabel={({ option }) => option.label}
      />
    </FormControl>
  );
}
