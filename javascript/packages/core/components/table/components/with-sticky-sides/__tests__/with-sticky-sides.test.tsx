import React from 'react';
import { render, screen } from '@testing-library/react';

import { withStickySides } from '../with-sticky-sides';

describe('withStickySides integration tests', () => {
  const MockRow = ({
    children,
    ...props
  }: { children: React.ReactNode } & Record<string, unknown>) => <tr {...props}>{children}</tr>;

  const StickySidesRow = withStickySides(MockRow);

  it('creates no sticky cells when enableStickySides is false', () => {
    render(
      <StickySidesRow
        enableStickySides={false}
        enableRowSelection={true}
        lastColumnIndex={3}
        scrollRatio={0}
        role="row"
      >
        <td>Cell 1</td>
        <td>Cell 2</td>
        <td>Cell 3</td>
        <td>Cell 4</td>
      </StickySidesRow>
    );

    expect(screen.queryByTestId(/^sticky-cell-/)).not.toBeInTheDocument();
  });

  it('creates sticky columns for table with row selection', () => {
    render(
      <StickySidesRow
        enableStickySides={true}
        enableRowSelection={true}
        lastColumnIndex={3}
        scrollRatio={0}
        role="row"
      >
        <td>Selection</td>
        <td>First Data</td>
        <td>Regular</td>
        <td>Config</td>
      </StickySidesRow>
    );

    // Should have sticky: selection (0) + first data (1) + config (3)
    expect(screen.getAllByTestId('sticky-cell-left-sticky')).toHaveLength(2);
    expect(screen.getAllByTestId('sticky-cell-right-sticky')).toHaveLength(1);
  });

  it('creates sticky columns for table without row selection', () => {
    render(
      <StickySidesRow
        enableStickySides={true}
        enableRowSelection={false}
        lastColumnIndex={2}
        scrollRatio={0}
        role="row"
      >
        <td>First Data</td>
        <td>Regular</td>
        <td>Config</td>
      </StickySidesRow>
    );

    // Should have sticky: first data (0) + config (2)
    expect(screen.getAllByTestId('sticky-cell-left-sticky')).toHaveLength(1);
    expect(screen.getAllByTestId('sticky-cell-right-sticky')).toHaveLength(1);
  });

  it('preserves original child content', () => {
    render(
      <StickySidesRow
        enableStickySides={true}
        enableRowSelection={true}
        lastColumnIndex={3}
        scrollRatio={0}
        role="row"
      >
        <td>Checkbox Column</td>
        <td>Name Column</td>
        <td>Status Column</td>
        <td>Config Button</td>
      </StickySidesRow>
    );

    expect(screen.getByText('Checkbox Column')).toBeInTheDocument();
    expect(screen.getByText('Name Column')).toBeInTheDocument();
    expect(screen.getByText('Status Column')).toBeInTheDocument();
    expect(screen.getByText('Config Button')).toBeInTheDocument();
  });
});
