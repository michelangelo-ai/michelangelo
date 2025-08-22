export function isFilterAlreadyApplied(currentColumnFilter: unknown, cellValue: unknown): boolean {
  if (!currentColumnFilter) return false;

  if (Array.isArray(currentColumnFilter)) {
    return currentColumnFilter.length === 1 && currentColumnFilter[0] === cellValue;
  }
  return currentColumnFilter === cellValue;
}
