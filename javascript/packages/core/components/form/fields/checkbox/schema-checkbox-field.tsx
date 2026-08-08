import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { CheckboxField } from './checkbox-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { CheckboxFieldConfig } from './types';

export function SchemaCheckboxField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing CheckboxFieldConfig
  const c = config as CheckboxFieldConfig;
  const validate = resolveValidation(c.validation);
  return (
    <CheckboxField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      description={c.description}
      caption={c.caption}
      options={c.options ?? []}
      validate={validate}
    />
  );
}
