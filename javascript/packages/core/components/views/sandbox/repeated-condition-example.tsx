import { ArrayFormGroup } from '#core/components/form/layout/array-form-group/array-form-group';
import { LayoutItemList } from '#core/components/form/layout/layout-item-list';

import type { FieldConfig, LayoutItem } from '#core/components/form/types/config-types';

/**
 * Each item's `detail` field is conditional on that same item's `enabled` field.
 * `when: 'items.enabled'` is entity-relative; `FormCondition` resolves it against the current
 * row's index so the same config doesn't leak across rows.
 */
export function RepeatedConditionExample() {
  return (
    <ArrayFormGroup rootFieldPath="items" groupLabel="Item" minItems={2}>
      {(indexedFieldPath) => {
        const fields: Record<string, FieldConfig | undefined> = {
          [`${indexedFieldPath}.enabled`]: { type: 'boolean', label: 'Show detail' },
          [`${indexedFieldPath}.detail`]: { type: 'string', label: 'Detail' },
        };
        const layout: LayoutItem[] = [
          `${indexedFieldPath}.enabled`,
          {
            type: 'condition',
            when: 'items.enabled',
            is: true,
            items: [`${indexedFieldPath}.detail`],
          },
        ];
        return <LayoutItemList items={layout} fields={fields} />;
      }}
    </ArrayFormGroup>
  );
}
