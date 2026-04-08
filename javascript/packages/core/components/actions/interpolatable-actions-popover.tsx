import { useMemo } from 'react';

import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { ActionsPopover } from './actions-popover';

import type { BasePopoverProps } from 'baseui/popover';
import type { ActionConfig, ActionConfigSchema, Data } from './types';

type InterpolatableActionsPopoverProps = {
  actions: ActionConfigSchema<Data>[];
  record: Data;
  popoverProps?: BasePopoverProps;
};

/**
 * Resolves interpolated action configs per-row before delegating to {@link ActionsPopover}.
 *
 * Table action render functions aren't React components and can't use hooks, so this thin
 * wrapper provides the per-row resolution boundary that the table view needs.
 */
export function InterpolatableActionsPopover({
  actions,
  record,
  ...rest
}: InterpolatableActionsPopoverProps) {
  const resolve = useInterpolationResolver();
  const resolved = useMemo(
    () => resolve(actions, { row: record }) as ActionConfig[],
    [resolve, actions, record]
  );

  return <ActionsPopover actions={resolved} record={record} {...rest} />;
}
