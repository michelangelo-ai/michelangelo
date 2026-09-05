import { formatEntityName } from '../use-entity-name';

describe('formatEntityName', () => {
  it('title-cases for "nav"', () => {
    expect(formatEntityName('trained models', 'nav')).toBe('Trained Models');
  });

  it('passes through for undefined casing', () => {
    expect(formatEntityName('trained models', undefined)).toBe('trained models');
  });

  it('passes through when casing is omitted entirely', () => {
    expect(formatEntityName('trained models')).toBe('trained models');
  });

  it('title-cases a single word', () => {
    expect(formatEntityName('pipelines', 'nav')).toBe('Pipelines');
  });

  it('returns an empty string unchanged', () => {
    expect(formatEntityName('', 'nav')).toBe('');
  });

  it('preserves acronyms already capitalized in the canonical name', () => {
    expect(formatEntityName('AI agents', 'nav')).toBe('AI Agents');
  });

  it('preserves hyphens rather than splitting them into separate words', () => {
    expect(formatEntityName('one-off predictions', 'nav')).toBe('One-off Predictions');
  });
});
