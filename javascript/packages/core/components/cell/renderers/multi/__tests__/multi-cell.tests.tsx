import { render, screen } from '@testing-library/react';

import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { MultiCell } from '../multi-cell';

describe('MultiCell', () => {
  it('should render dash when no data is available', () => {
    render(
      <MultiCell
        column={{
          id: 'multi',
          items: [{ id: 'nonExistent.path' }, { id: 'another.nonExistent.path' }],
        }}
        record={{ metadata: { name: 'Test Pipeline' }, spec: { revisionId: 'rev-123', description: 'Test Description' } }}
        value={undefined}
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('should render multiple items with default renderer', () => {
    render(
      <MultiCell
        column={{
          id: 'multi',
          items: [{ id: 'metadata.name' }, { id: 'spec.revisionId' }],
        }}
        record={{ metadata: { name: 'Test Pipeline' }, spec: { revisionId: 'rev-123', description: 'Test Description' } }}
        value={undefined}
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByText('Test Pipeline')).toBeInTheDocument();
    expect(screen.getByText('rev-123')).toBeInTheDocument();
  });

  it('should render icon when provided', () => {
    render(
      <MultiCell
        column={{
          id: 'multi',
          icon: 'check',
          items: [{ id: 'metadata.name' }, { id: 'spec.revisionId' }],
        }}
        record={{ metadata: { name: 'Test Pipeline' }, spec: { revisionId: 'rev-123', description: 'Test Description' } }}
        value={undefined}
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getAllByTitle('Check').length).toBeGreaterThan(0);
  });

  it('should use accessor when provided instead of id', () => {
    render(
      <MultiCell
        column={{
          id: 'multi',
          items: [{ id: 'metadata.name', accessor: 'spec.description' }, { id: 'spec.revisionId' }],
        }}
        record={{ metadata: { name: 'Test Pipeline' }, spec: { revisionId: 'rev-123', description: 'Test Description' } }}
        value={undefined}
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByText('Test Description')).toBeInTheDocument();
    expect(screen.getByText('rev-123')).toBeInTheDocument();
  });

  it('should handle empty items array', () => {
    render(
      <MultiCell
        column={{
          id: 'multi',
          items: [],
        }}
        record={{ metadata: { name: 'Test Pipeline' }, spec: { revisionId: 'rev-123', description: 'Test Description' } }}
        value={undefined}
      />,
      { wrapper: getIconProviderWrapper() }
    );

    expect(screen.getByText('—')).toBeInTheDocument();
  });
});
