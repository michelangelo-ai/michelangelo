import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { Row } from '../row';

describe('Row', () => {
  const mockItems = [
    { id: 'name', label: 'Name', hideEmpty: true },
    { id: 'age', label: 'Age', hideEmpty: false },
    { id: 'email', label: 'Email', hideEmpty: true },
  ];

  const mockRecord = {
    name: 'John Doe',
    age: 30,
    email: undefined,
  };

  it('renders skeleton loaders when loading is true', () => {
    render(<Row items={mockItems} loading={true} />, buildWrapper([getRouterWrapper()]));
    const skeletons = screen.getAllByTestId('loading');
    expect(skeletons).toHaveLength(mockItems.length);
  });

  it('filters out empty items when hideEmpty is true', () => {
    render(<Row items={mockItems} record={mockRecord} />, buildWrapper([getRouterWrapper()]));
    expect(screen.getByText('John Doe')).toBeInTheDocument();
    expect(screen.getByText('30')).toBeInTheDocument();
    expect(screen.queryByText('Email')).not.toBeInTheDocument();
  });

  it('renders all items when hideEmpty is false', () => {
    const itemsWithoutHideEmpty = mockItems.map((item) => ({ ...item, hideEmpty: false }));
    render(
      <Row items={itemsWithoutHideEmpty} record={mockRecord} />,
      buildWrapper([getRouterWrapper()])
    );
    expect(screen.getByText('John Doe')).toBeInTheDocument();
    expect(screen.getByText('30')).toBeInTheDocument();
    expect(screen.getByText('Email')).toBeInTheDocument();
  });

  it('applies custom overrides correctly', () => {
    const CustomContainer = ({ children, ...props }) => (
      <div data-testid="custom-container" {...props}>
        {children}
      </div>
    );

    const overrides = {
      RowContainer: {
        component: CustomContainer,
      },
    };

    render(<Row items={mockItems} overrides={overrides} />, buildWrapper([getRouterWrapper()]));
    expect(screen.getByTestId('custom-container')).toBeInTheDocument();
  });

  it('handles nested record data correctly', () => {
    const itemsWithAccessor = [{ id: 'user', accessor: 'user.name', label: 'User Name' }];
    const nestedRecord = {
      user: {
        name: 'John Doe',
      },
    };

    render(
      <Row items={itemsWithAccessor} record={nestedRecord} />,
      buildWrapper([getRouterWrapper()])
    );
    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });
});
