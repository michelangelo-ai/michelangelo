import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { SelectField } from './select-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { SelectFieldConfig } from './types';

export function SchemaSelectField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing SelectFieldConfig
  const c = config as SelectFieldConfig;
  const validate = resolveValidation(c.validation);

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
      options={c.options ?? []}
      multi={c.multi}
      clearable={c.clearable}
      searchable={c.searchable}
      creatable={c.creatable}
      isLoading={c.isLoading}
      visibleOptionLimit={c.visibleOptionLimit}
      validate={validate}
    />
  );
}
