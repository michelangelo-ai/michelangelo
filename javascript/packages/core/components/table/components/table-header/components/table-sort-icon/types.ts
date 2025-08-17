export type TableSortIconProps = {
  column: {
    getIsSorted: () => false | 'asc' | 'desc';
  };
};
