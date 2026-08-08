import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { StringField } from './string-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { StringFieldConfig } from './types';

/** Schema-aware wrapper for StringField that maps config to component props. */
export function SchemaStringField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing StringFieldConfig
  const c = config as StringFieldConfig;
  const validate = resolveValidation(c.validation);

  return (
    <StringField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
      multi={c.multi}
      validate={validate}
    />
  );
}
