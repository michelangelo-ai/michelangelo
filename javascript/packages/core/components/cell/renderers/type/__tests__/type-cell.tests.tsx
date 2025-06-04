import { render, screen } from '@testing-library/react';

import { TypeCell } from '../type-cell';
import { typeCellToString } from '../type-cell-to-string';

describe('TypeCell', () => {
  it('should render type text with default styling when no custom maps are provided', () => {
    render(<TypeCell column={{ id: 'type' }} record={{}} value="FEATURE_TYPE_CATEGORICAL" />);

    expect(screen.getByText('Categorical')).toBeInTheDocument();
  });

  it('should render custom type text when provided in typeTextMap', () => {
    render(
      <TypeCell
        column={{
          id: 'type',
          typeTextMap: {
            FEATURE_TYPE_CATEGORICAL: 'Custom Categorical Text',
          },
        }}
        record={{}}
        value="FEATURE_TYPE_CATEGORICAL"
      />
    );

    expect(screen.getByText('Custom Categorical Text')).toBeInTheDocument();
  });

  it('should render empty value correctly', () => {
    const { container } = render(<TypeCell column={{ id: 'type' }} record={{}} value="" />);
    expect(container).toBeEmptyDOMElement();
  });

  describe('toString', () => {
    it('should return empty string for empty value', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        value: '',
      });

      expect(result).toBe('');
    });

    it('should return custom text from typeTextMap when available', () => {
      const result = typeCellToString({
        column: {
          id: 'type',
          typeTextMap: {
            FEATURE_TYPE_CATEGORICAL: 'Custom Categorical Text',
          },
        },
        value: 'FEATURE_TYPE_CATEGORICAL',
      });

      expect(result).toBe('Custom Categorical Text');
    });

    it('should handle FORMAT prefix correctly', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        value: 'DATA_FORMAT_CSV',
      });

      expect(result).toBe('Csv');
    });

    it('should handle KIND prefix correctly', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        value: 'MODEL_KIND_TENSORFLOW',
      });

      expect(result).toBe('Tensorflow');
    });

    it('should handle TYPE prefix correctly', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        value: 'FEATURE_TYPE_CATEGORICAL',
      });

      expect(result).toBe('Categorical');
    });

    it('should handle non-prefixed values correctly', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        value: 'SIMPLE_TYPE',
      });

      expect(result).toBe('Simple type');
    });

    it('should handle numeric values gracefully', () => {
      const result = typeCellToString({
        column: { id: 'type' },
        // @ts-expect-error - we want to test the function with a non-string input
        value: 123,
      });

      expect(result).toEqual(123);
    });
  });
});
