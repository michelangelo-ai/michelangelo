import { SchemaField } from '#core/components/form/fields/schema-field';
import { LayoutItemRenderer } from './layout-item-renderer';

import type { FieldConfig, LayoutItem } from '#core/components/form/types/config-types';

/** Walks a layout tree, rendering bare strings as fields and objects as layout nodes. */
export function LayoutItemList({
  items,
  fields,
}: {
  items: LayoutItem[];
  fields: Record<string, FieldConfig | undefined>;
}) {
  return (
    <>
      {items.map((item, index) =>
        typeof item === 'string' ? (
          <SchemaField key={item} fieldPath={item} config={fields[item]} />
        ) : (
          <LayoutItemRenderer key={index} config={item} fields={fields} />
        )
      )}
    </>
  );
}
