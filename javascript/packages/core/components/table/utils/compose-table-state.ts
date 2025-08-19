import type { ControlledTableState, TableState } from '../types/table-types';

const STATE_NAME_TO_STATE_SETTER_NAME = {
  globalFilter: 'setGlobalFilter',
  columnFilters: 'setColumnFilters',
  pagination: 'setPagination',
  sorting: 'setSorting',
  columnOrder: 'setColumnOrder',
  columnVisibility: 'setColumnVisibility',
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
export function composeTableState(combinedState: Partial<ControlledTableState>): {
  initialState: Partial<TableState>;
  state: Partial<ControlledTableState>;
} {
  const state = {};
  const initialState = {};

  Object.entries(STATE_NAME_TO_STATE_SETTER_NAME).forEach(([propertyName, setterName]) => {
    if (setterName in combinedState) {
      if (!(propertyName in combinedState)) {
        console.warn(
          `Controlled state setter ${setterName} must be accompanied by property ${propertyName}`
        );
      }

      state[propertyName] = combinedState[propertyName] as TableState[keyof TableState];
      state[setterName] = combinedState[
        setterName
      ] as ControlledTableState[keyof ControlledTableState];
    } else if (propertyName in combinedState) {
      initialState[propertyName] = combinedState[propertyName] as TableState[keyof TableState];
    }
  });

  return { initialState, state };
}
