import { SchemaStringField } from './fields/string/schema-string-field';

import type { FieldRenderer, FieldType, LayoutRenderer } from './types';

export const FIELD_RENDERERS: Partial<Record<FieldType, FieldRenderer>> = {
  string: SchemaStringField,
};

export const LAYOUT_RENDERERS: Record<string, LayoutRenderer> = {};
