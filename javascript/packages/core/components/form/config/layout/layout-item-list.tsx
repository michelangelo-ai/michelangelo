import { SchemaField } from '../fields/schema-field';
import { LayoutItemRenderer } from './layout-item-renderer';

import type { FieldConfig, LayoutItem } from '../types';

export function LayoutItemList({
  items,
  entities,
}: {
  items: LayoutItem[];
  entities: Record<string, FieldConfig>;
}) {
  return (
    <>
      {items.map((item, index) =>
        typeof item === 'string' ? (
          <SchemaField key={item} fieldPath={item} config={entities[item]} />
        ) : (
          <LayoutItemRenderer key={index} config={item} entities={entities} />
        )
      )}
    </>
  );
}
