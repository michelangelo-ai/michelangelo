import { SelectField } from './select-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { SelectFieldConfig } from './types';

export function SchemaSelectField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing SelectFieldConfig
  const c = config as SelectFieldConfig;
  const options = c.options ?? [];

  if (c.multi) {
    return (
      <SelectField
        name={name}
        label={c.label}
        required={c.required}
        disabled={c.disabled}
        readOnly={c.readOnly}
        placeholder={c.placeholder}
        description={c.description}
        caption={c.caption}
        options={options}
        multi
        clearable={c.clearable}
        searchable={c.searchable}
        creatable={c.creatable}
        isLoading={c.isLoading}
        visibleOptionLimit={c.visibleOptionLimit}
      />
    );
  }

  return (
    <SelectField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
      options={options}
      clearable={c.clearable}
      searchable={c.searchable}
      creatable={c.creatable}
      isLoading={c.isLoading}
      visibleOptionLimit={c.visibleOptionLimit}
    />
  );
}
