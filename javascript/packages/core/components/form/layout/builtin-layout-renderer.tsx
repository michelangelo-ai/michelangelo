import { FormGrid } from '#core/components/form/layout/form-grid/form-grid';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { LayoutItemList } from './layout-item-list';

import type { BuiltinLayoutConfig, FieldConfig } from '#core/components/form/types/config-types';

export function BuiltinLayoutRenderer({
  config,
  fields,
}: {
  config: BuiltinLayoutConfig;
  fields: Record<string, FieldConfig>;
}) {
  const renderChildren = <LayoutItemList items={config.items} fields={fields} />;

  switch (config.type) {
    case 'group':
      return (
        <FormGroup
          title={config.label}
          description={config.description}
          tooltip={config.tooltip}
          collapsible={config.collapsible}
        >
          {renderChildren}
        </FormGroup>
      );
    case 'row':
      return (
        <FormRow name={config.name} span={config.span}>
          {renderChildren}
        </FormRow>
      );
    case 'grid':
      return <FormGrid>{renderChildren}</FormGrid>;
  }
}
