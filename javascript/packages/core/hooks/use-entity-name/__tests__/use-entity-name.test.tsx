import { render, screen } from '@testing-library/react';

import { DisplayProvider } from '#core/providers/display-provider/display-provider';
import { formatEntityName, useEntityName } from '../use-entity-name';

function EntityNameProbe({ name, casing }: { name: string; casing?: 'nav' | 'content' }) {
  return <span>{useEntityName(name, casing)}</span>;
}

describe('useEntityName', () => {
  it('title-cases the name inside a nav DisplayProvider', () => {
    render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="trained models" />
      </DisplayProvider>
    );
    expect(screen.getByText('Trained Models')).toBeInTheDocument();
  });

  it('passes the name through unchanged inside a content DisplayProvider', () => {
    render(
      <DisplayProvider type="content">
        <EntityNameProbe name="trained models" />
      </DisplayProvider>
    );
    expect(screen.getByText('trained models')).toBeInTheDocument();
  });

  it('passes the name through unchanged with no ambient DisplayProvider', () => {
    render(<EntityNameProbe name="trained models" />);
    expect(screen.getByText('trained models')).toBeInTheDocument();
  });

  it('the same entity name renders differently depending on the surrounding DisplayProvider', () => {
    const { rerender } = render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="trained models" />
      </DisplayProvider>
    );
    expect(screen.getByText('Trained Models')).toBeInTheDocument();

    rerender(
      <DisplayProvider type="content">
        <EntityNameProbe name="trained models" />
      </DisplayProvider>
    );
    expect(screen.queryByText('Trained Models')).not.toBeInTheDocument();
    expect(screen.getByText('trained models')).toBeInTheDocument();
  });

  it('an explicit casing override beats the ambient DisplayProvider', () => {
    render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="trained models" casing="content" />
      </DisplayProvider>
    );
    expect(screen.getByText('trained models')).toBeInTheDocument();
    expect(screen.queryByText('Trained Models')).not.toBeInTheDocument();
  });

  it('preserves acronyms already capitalized in the canonical name', () => {
    render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="AI agents" />
      </DisplayProvider>
    );
    expect(screen.getByText('AI Agents')).toBeInTheDocument();
  });

  it('preserves hyphens rather than splitting them into separate words', () => {
    render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="one-off predictions" />
      </DisplayProvider>
    );
    expect(screen.getByText('One-off Predictions')).toBeInTheDocument();
  });

  it('a nested DisplayProvider overrides its ancestor for the subtree inside it', () => {
    render(
      <DisplayProvider type="nav">
        <EntityNameProbe name="trained models" />
        <DisplayProvider type="content">
          <EntityNameProbe name="pipelines" />
        </DisplayProvider>
      </DisplayProvider>
    );
    expect(screen.getByText('Trained Models')).toBeInTheDocument();
    expect(screen.getByText('pipelines')).toBeInTheDocument();
  });
});

describe('formatEntityName', () => {
  it('title-cases for "nav"', () => {
    expect(formatEntityName('trained models', 'nav')).toBe('Trained Models');
  });

  it('passes through for "content"', () => {
    expect(formatEntityName('trained models', 'content')).toBe('trained models');
  });

  it('passes through for undefined casing', () => {
    expect(formatEntityName('trained models', undefined)).toBe('trained models');
  });

  it('title-cases a single word', () => {
    expect(formatEntityName('pipelines', 'nav')).toBe('Pipelines');
  });

  it('returns an empty string unchanged', () => {
    expect(formatEntityName('', 'nav')).toBe('');
  });
});
