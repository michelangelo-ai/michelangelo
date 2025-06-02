import { render, screen } from '@testing-library/react';

import { BooleanCell } from '../boolean-cell';

describe('BooleanCell', () => {
  test('Renders nothing for empty value', () => {
    render(
      <BooleanCell
        column={{ id: 'spec.bool', label: 'ColumnLabel' }}
        record={{ spec: { bool: false } }}
        value={false}
      />
    );

    expect(screen.queryByText('ColumnLabel')).toBeNull();
  });

  test('Renders nothing for falsy value', () => {
    render(
      <BooleanCell column={{ id: 'spec.bool' }} record={{ spec: { bool: false } }} value={false} />
    );

    expect(screen.queryByText('ColumnLabel')).toBeNull();
  });

  test('Renders column label for truthy value', () => {
    render(
      <BooleanCell
        column={{ id: 'spec.bool', label: 'ColumnLabel' }}
        record={{ spec: { bool: true } }}
        value={true}
      />
    );

    expect(screen.getByText('ColumnLabel')).toBeInTheDocument();
  });
});

describe('BooleanTextRenderer', () => {
  test('Renders nothing for empty value', () => {
    // @ts-expect-error - intentionally testing with invalid props
    expect(BooleanCell.toString({})).toBeFalsy();
  });

  test('Renders nothing for falsy value', () => {
    expect(BooleanCell.toString({ value: false, column: { id: 'spec.bool' } })).toBeFalsy();
  });

  test('Renders True for truthy value without label', () => {
    expect(BooleanCell.toString({ value: true, column: { id: 'spec.bool' } })).toBe('True');
  });

  test('Renders column label for truthy value', () => {
    expect(
      BooleanCell.toString({ value: true, column: { id: 'spec.bool', label: 'ColumnLabel' } })
    ).toEqual('ColumnLabel');
  });
});
