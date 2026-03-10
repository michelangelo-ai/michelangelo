import { useEffect, useMemo } from 'react';
import { filterOptions as defaultFilterOptions, OnChangeParams, Select } from 'baseui/select';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { buildSelectOverrides } from './build-select-overrides';
import { formatSelectedValue } from './format-selected-value';

import type { SelectFieldProps, SelectOption } from './types';

/**
 * BaseUI requires option IDs to be string | number. To support arbitrary ID types
 * (including objects), we serialize IDs to strings for BaseUI and resolve them back
 * to their originals for form state via a lookup map.
 */
const serialize = JSON.stringify;

function buildOptionLookup<V>(options: SelectOption<V>[]) {
  const lookup = new Map<string, SelectOption<V>>();
  const normalizedOptions = options.map((opt) => {
    const key = serialize(opt.id);
    lookup.set(key, opt);
    return { id: key, label: opt.label, disabled: opt.disabled };
  });
  return { normalizedOptions, lookup };
}

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
  placeholder,
  options,
  maxOptions,
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
  });

  const { normalizedOptions, lookup } = useMemo(() => buildOptionLookup(options), [options]);

  const resolveOriginalId = (serializedKey: string): V => {
    const original = lookup.get(serializedKey);
    return (original ? original.id : serializedKey) as V;
  };

  // Clear field value when it doesn't match any available option.
  // Deps intentionally exclude input/multi to avoid re-running on every value change,
  // which would loop since we call onChange inside.
  useEffect(() => {
    if (isLoading || creatable) return;

    const currentValue = input.value;
    if (!currentValue || (Array.isArray(currentValue) && currentValue.length === 0)) return;

    if (multi) {
      const values = currentValue as V[];
      const validValues = values.filter((v) => lookup.has(serialize(v)));
      if (validValues.length !== values.length) {
        input.onChange(validValues as V | V[]);
      }
    } else if (!lookup.has(serialize(currentValue))) {
      input.onChange('' as V | V[]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookup, isLoading]);

  const handleChange = (params: OnChangeParams) => {
    if (multi) {
      const values = params.value.map((item) => resolveOriginalId(String(item.id)));
      input.onChange(values as V | V[]);
    } else if (params.value.length > 0) {
      input.onChange(resolveOriginalId(String(params.value[0].id)) as V | V[]);
    } else {
      input.onChange('' as V | V[]);
    }
  };

  const selectedValue = useMemo(() => {
    const items: Array<{ id: string; label: string; disabled?: boolean }> = [];
    for (const item of formatSelectedValue(input.value)) {
      const key = serialize(item);
      const matched = lookup.get(key);
      if (matched) {
        items.push({ id: key, label: matched.label, disabled: matched.disabled });
      } else if (creatable) {
        items.push({ id: key, label: String(item) });
      }
    }
    return items;
  }, [input.value, lookup, creatable]);

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
        value={selectedValue}
        options={normalizedOptions}
        onChange={handleChange}
        onBlur={input.onBlur}
        placeholder={!disabled && !readOnly ? placeholder : ''}
        disabled={disabled}
        clearable={!disabled && !readOnly && clearable}
        searchable={searchable}
        multi={multi}
        overrides={buildSelectOverrides(name, disabled, readOnly)}
        creatable={creatable}
        filterOptions={(options, filterValue, excludeOptions, newProps) =>
          defaultFilterOptions(options, filterValue, excludeOptions, newProps).slice(0, maxOptions)
        }
        isLoading={isLoading}
        // Modified getOptionLabel in BaseWeb Select to avoid adding "Create" prefix on user input for
        // creatable dropdowns, as creation typically occurs during form submission. The prefix is misleading.
        getOptionLabel={({ option }) => option.label}
      />
    </FormControl>
  );
}
