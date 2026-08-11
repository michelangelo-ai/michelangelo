import { SchemaStringField } from './fields/string/schema-string-field';

import type { FieldRenderer, FieldType } from './types/config-types';

/** Built-in field renderer registry. FormProvider renderers take priority over these. */
export const FIELD_RENDERERS: Partial<Record<FieldType, FieldRenderer>> = {
  string: SchemaStringField,
};
