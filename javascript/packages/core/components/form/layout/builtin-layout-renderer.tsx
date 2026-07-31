import { FormGrid } from '#core/components/form/layout/form-grid/form-grid';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { LayoutItemList } from './layout-item-list';

import type { BuiltinLayoutConfig, FieldConfig } from '#core/components/form/types';

export function BuiltinLayoutRenderer({
  config,
  entities,
}: {
  config: BuiltinLayoutConfig;
  entities: Record<string, FieldConfig>;
}) {
  const renderChildren = <LayoutItemList items={config.items} entities={entities} />;

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
