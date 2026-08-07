import { FormGrid } from '#core/components/form/layout/form-grid/form-grid';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { LayoutItemList } from './layout-item-list';

import type { FieldConfig, LayoutConfig } from '#core/components/form/types/config-types';

/** Renders a layout config by dispatching to the matching layout component. */
export function LayoutRenderer({
  config,
  fields,
}: {
  config: LayoutConfig;
  fields: Record<string, FieldConfig | undefined>;
}) {
  const children = <LayoutItemList items={config.items} fields={fields} />;

  switch (config.type) {
    case 'group':
      return (
        <FormGroup
          title={config.title}
          description={config.description}
          tooltip={config.tooltip}
          collapsible={config.collapsible}
        >
          {children}
        </FormGroup>
      );
    case 'row':
      return (
        <FormRow name={config.name} span={config.span}>
          {children}
        </FormRow>
      );
    case 'grid':
      return <FormGrid>{children}</FormGrid>;
  }
}
