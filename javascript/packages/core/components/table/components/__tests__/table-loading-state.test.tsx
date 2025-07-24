import { render, screen } from '@testing-library/react';

import { TableLoadingState } from '../table-loading-state';

describe('TableLoadingState', () => {
  it('should render 3 skeleton rows', () => {
    render(<TableLoadingState />);
    expect(screen.getAllByRole('row')).toHaveLength(3);
  });
});
