import React from 'react';

import { ControlledTableState, TableState } from '../../types/table-types';

/**
 * Provides controlled/uncontrolled state management for row selection enabling/disabling
 *
 * @remarks
 * This hook handles both controlled and uncontrolled row selection state:
 *
 * **Controlled mode**: When `state.setRowSelectionEnabled` is provided,
 * uses the provided state and setter directly.
 *
 * **Uncontrolled mode**: When only `initialState.rowSelectionEnabled` is provided,
 * manages internal state that can be updated via the returned setter.
 *
 * This is necessary because TanStack Table doesn't provide built-in state management
 * for dynamically enabling/disabling row selection.
 */
export function useRowSelectionState({
  state,
  initialState,
}: {
  state: Partial<ControlledTableState>;
  initialState: Partial<TableState>;
}): { enableRowSelection: boolean; setRowSelectionEnabled: (enabled: boolean) => void } {
  const [internalRowSelectionEnabled, setInternalRowSelectionEnabled] = React.useState(
    initialState.rowSelectionEnabled
  );

  const enableRowSelection = state.setRowSelectionEnabled
    ? state.rowSelectionEnabled
    : internalRowSelectionEnabled;

  return {
    enableRowSelection: enableRowSelection ?? false,
    setRowSelectionEnabled: state.setRowSelectionEnabled ?? setInternalRowSelectionEnabled,
  };
}
