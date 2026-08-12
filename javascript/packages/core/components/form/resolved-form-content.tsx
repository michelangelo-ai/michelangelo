import { useRef } from 'react';
import { useFormState } from 'react-final-form';
import { isEqual } from 'lodash';

import { LayoutItemList } from '#core/components/form/layout/layout-item-list';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { clearUnresolvedInterpolations } from '#core/interpolation/utils/clear-unresolved-interpolations';
import { hasInterpolationProperty } from '#core/interpolation/utils/has-interpolation-property';

import type { FieldConfig, LayoutItem } from '#core/components/form/types/config-types';

type ResolvedFormContentProps = {
  layout: LayoutItem[];
  fields: Record<string, FieldConfig | undefined>;
};

/**
 * Renders a form's layout, resolving interpolated field configs against the current
 * form values when any are present.
 *
 * Interpolation depends on router context (via `useInterpolationResolver`), so the
 * interpolated path is split into its own component: mounting it only when the config
 * actually needs it keeps forms with no interpolations free of that dependency.
 */
export function ResolvedFormContent({ layout, fields }: ResolvedFormContentProps) {
  if (!hasInterpolationProperty(fields) && !hasInterpolationProperty(layout)) {
    return <LayoutItemList items={layout} fields={fields} />;
  }

  return <InterpolatedFormContent layout={layout} fields={fields} />;
}

// Extracted to keep interpolation's router-dependent hooks out of the non-interpolated path above.
// Not substantial enough to warrant a separate file.
// eslint-disable-next-line react/no-multi-comp
function InterpolatedFormContent({ layout, fields }: ResolvedFormContentProps) {
  const { values, initialValues } = useFormState({
    subscription: { values: true, initialValues: true },
  });
  const resolver = useInterpolationResolver();

  const previousFields = useRef<Record<string, FieldConfig | undefined>>({});
  const previousLayout = useRef<LayoutItem[] | undefined>(undefined);

  // TODO: Add excludeProperty callback when SelectFieldConfig gains queryConfig,
  // to skip resolving large generated options arrays (see doesSchemaRequireInterpolation in studio-web)
  const resolved = clearUnresolvedInterpolations(
    resolver({ fields, layout }, { page: values, initialValues })
  );

  const stabilizedFields: Record<string, FieldConfig | undefined> = {};
  for (const key of Object.keys(resolved.fields)) {
    const resolvedField = resolved.fields[key];
    const previousField = previousFields.current[key];
    stabilizedFields[key] = isEqual(resolvedField, previousField) ? previousField : resolvedField;
  }
  previousFields.current = stabilizedFields;

  const stabilizedLayout = isEqual(resolved.layout, previousLayout.current)
    ? previousLayout.current!
    : resolved.layout;
  previousLayout.current = stabilizedLayout;

  return <LayoutItemList items={stabilizedLayout} fields={stabilizedFields} />;
}
