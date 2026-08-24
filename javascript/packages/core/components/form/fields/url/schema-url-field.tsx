import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { UrlField } from './url-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { UrlFieldConfig } from './types';

export function SchemaUrlField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing UrlFieldConfig
  const c = config as UrlFieldConfig;
  const validate = resolveValidation(c.validation);
  return (
    <UrlField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
      urlName={c.urlName}
      validate={validate}
    />
  );
}
