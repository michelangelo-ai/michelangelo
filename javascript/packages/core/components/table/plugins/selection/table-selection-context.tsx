import * as React from 'react';

import type { TableSelectionContext as TableSelectionContextType } from './types';

export const TableSelectionContext = React.createContext<TableSelectionContextType>({
  selectedRows: [],
  selectionEnabled: false,
  setSelectionEnabled: () => null,
  toggleAllRowsSelected: () => null,
  getIsAllRowsSelected: () => false,
  getIsSomeRowsSelected: () => false,
});

/**
 * Accesses the table selection state and controls for managing row selection.
 *
 * This hook must be used within a Table component that has row selection enabled.
 * It provides access to the currently selected rows and functions to control selection.
 *
 * @returns Table selection context containing:
 *   - `selectedRows`: Array of currently selected row data
 *   - `selectionEnabled`: Whether row selection is currently enabled
 *   - `setSelectionEnabled`: Function to enable/disable row selection
 *   - `toggleAllRowsSelected`: Function to select/deselect all rows
 *   - `getIsAllRowsSelected`: Function to check if all rows are selected
 *   - `getIsSomeRowsSelected`: Function to check if some (but not all) rows are selected
 *
 * @example
 * ```typescript
 * function BulkActionsToolbar() {
 *   const {
 *     selectedRows,
 *     selectionEnabled,
 *     setSelectionEnabled,
 *     toggleAllRowsSelected,
 *     getIsSomeRowsSelected
 *   } = useTableSelectionContext();
 *
 *   return (
 *     <div>
 *       <button onClick={() => setSelectionEnabled(!selectionEnabled)}>
 *         {selectionEnabled ? 'Disable' : 'Enable'} Selection
 *       </button>
 *
 *       {selectionEnabled && (
 *         <>
 *           <span>{selectedRows.length} rows selected</span>
 *           <button onClick={toggleAllRowsSelected}>
 *             {getIsSomeRowsSelected() ? 'Select All' : 'Deselect All'}
 *           </button>
 *           <button disabled={selectedRows.length === 0}>
 *             Delete Selected
 *           </button>
 *         </>
 *       )}
 *     </div>
 *   );
 * }
 * ```
 */
export const useTableSelectionContext = () => React.useContext(TableSelectionContext);
