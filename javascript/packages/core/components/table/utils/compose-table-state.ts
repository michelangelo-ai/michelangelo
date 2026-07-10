import type { ControlledTableState, InputTableState, TableState } from '../types/table-types';

const STATE_NAME_TO_STATE_SETTER_NAME = {
  globalFilter: 'setGlobalFilter',
  columnFilters: 'setColumnFilters',
  pagination: 'setPagination',
  sorting: 'setSorting',
  columnOrder: 'setColumnOrder',
  columnVisibility: 'setColumnVisibility',
  rowSelection: 'setRowSelection',
  rowSelectionEnabled: 'setRowSelectionEnabled',
  grouping: 'setGrouping',
} as const;

/**
 * Uses a partial state object to construct appropriate state configuration for
 * the Table component.
 *
 * @param combinedState The combined state object, with setters and values
 *
 * @returns An object containing:
 * - `initialState`: {@link TableState} Values that will be initialized to the provided value and managed
 *  by the table during runtime.
 *
 * - `state`: {@link ControlledTableState} Values that will not be managed by the table and have an
 *  associated setter function.
 *
 * @remarks
 * A property will be considered controlled if the input state object contains a setter.
 */
export function composeTableState(combinedState: InputTableState): {
  initialState: Partial<TableState>;
  state: Partial<ControlledTableState>;
} {
  const state: Partial<ControlledTableState> = {};
  const initialState: Partial<TableState> = {};

  // cast: Object.keys widens to string; single cast here instead of per-assignment
  const stateKeys = Object.keys(
    STATE_NAME_TO_STATE_SETTER_NAME
  ) as (keyof typeof STATE_NAME_TO_STATE_SETTER_NAME)[];

  for (const propertyName of stateKeys) {
    const setterName = STATE_NAME_TO_STATE_SETTER_NAME[propertyName];

    if (setterName in combinedState) {
      if (!(propertyName in combinedState)) {
        console.warn(
          `Controlled state setter ${setterName} must be accompanied by property ${propertyName}`
        );
      }

      // cast: TS correlated-union limitation (microsoft/TypeScript#30581) — propertyName
      // is a state key but TS can't narrow the value type to match the specific key
      (state as Record<string, unknown>)[propertyName] = combinedState[propertyName];
      // cast: same correlated-union limitation for the setter assignment
      (state as Record<string, unknown>)[setterName] = combinedState[setterName];
    } else if (propertyName in combinedState) {
      // cast: same correlated-union limitation for initial state assignment
      (initialState as Record<string, unknown>)[propertyName] = combinedState[propertyName];
    }
  }

  return { initialState, state };
}
