import { render, screen } from '@testing-library/react';

import { StateCell } from '../state-cell';
import { stateToString } from '../state-cell-to-string';

describe('StateCell', () => {
  it('should render state text with default color when no custom maps are provided', () => {
    render(<StateCell column={{ id: 'state' }} record={{}} value="PIPELINE_STATE_BUILDING" />);

    expect(screen.getByText('Building')).toBeInTheDocument();
  });

  it('should render custom state text when provided in stateTextMap', () => {
    render(
      <StateCell
        column={{
          id: 'state',
          stateTextMap: {
            PIPELINE_STATE_BUILDING: 'Custom Building Text',
          },
        }}
        record={{}}
        value="PIPELINE_STATE_BUILDING"
      />
    );

    expect(screen.getByText('Custom Building Text')).toBeInTheDocument();
  });

  it('should render empty value correctly', () => {
    const { container } = render(<StateCell column={{ id: 'state' }} record={{}} value="" />);
    expect(container).toBeEmptyDOMElement();
  });

  it('should render invalid state as Queued', () => {
    render(<StateCell column={{ id: 'state' }} record={{}} value="PIPELINE_STATE_INVALID" />);

    expect(screen.getByText('Queued')).toBeInTheDocument();
  });

  describe('toString', () => {
    it('should return empty string for empty value', () => {
      const result = stateToString({
        column: { id: 'state' },
        value: '',
      });

      expect(result).toBe('');
    });

    it('should return custom text from stateTextMap when available', () => {
      const result = stateToString({
        column: {
          id: 'state',
          stateTextMap: {
            PIPELINE_STATE_BUILDING: 'Custom Building Text',
          },
        },
        value: 'PIPELINE_STATE_BUILDING',
      });

      expect(result).toBe('Custom Building Text');
    });

    it('should return Queued for invalid states', () => {
      const result = stateToString({
        column: { id: 'state' },
        value: 'PIPELINE_STATE_INVALID',
      });

      expect(result).toBe('Queued');
    });

    it('should return the mapped text for a numeric enum value', () => {
      const result = stateToString({
        column: { id: 'state', stateTextMap: { 6: 'Paused' } },
        // cast: proto enums decode to numbers at runtime even though the cell is typed as string
        value: 6 as unknown as string,
      });

      expect(result).toBe('Paused');
    });

    it('should not throw for a numeric enum value missing from stateTextMap', () => {
      const result = stateToString({
        column: { id: 'state', stateTextMap: { 1: 'Running' } },
        // cast: proto enums decode to numbers at runtime even though the cell is typed as string
        value: 6 as unknown as string,
      });

      expect(result).toBe('6');
    });

    it('should return sentence case for unknown states', () => {
      const result = stateToString({
        column: { id: 'state' },
        value: 'PIPELINE_STATE_UNKNOWN_STATE',
      });

      expect(result).toBe('Unknown state');
    });
  });
});
