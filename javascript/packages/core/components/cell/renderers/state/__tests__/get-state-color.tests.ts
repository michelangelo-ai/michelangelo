import { COLOR } from '#core/components/tag/constants';
import { getStateColor } from '../get-state-color';

describe('getStateColor', () => {
  it('should return gray for empty value', () => {
    expect(getStateColor('')).toBe(COLOR.gray);
  });

  it('should return red for error states', () => {
    expect(getStateColor('PIPELINE_STATE_ERROR')).toBe(COLOR.red);
    expect(getStateColor('ANY_STATE_ERROR')).toBe(COLOR.red);
  });

  it('should return green for success states', () => {
    expect(getStateColor('PIPELINE_STATE_SUCCESS')).toBe(COLOR.green);
    expect(getStateColor('ANY_STATE_SUCCESS')).toBe(COLOR.green);
  });

  it('should return blue for running states', () => {
    expect(getStateColor('PIPELINE_STATE_RUNNING')).toBe(COLOR.blue);
    expect(getStateColor('ANY_STATE_RUNNING')).toBe(COLOR.blue);
  });

  it('should return gray for invalid states', () => {
    expect(getStateColor('PIPELINE_STATE_INVALID')).toBe(COLOR.gray);
    expect(getStateColor('ANY_STATE_INVALID')).toBe(COLOR.gray);
  });

  it('should return gray for unknown states', () => {
    expect(getStateColor('PIPELINE_STATE_UNKNOWN')).toBe(COLOR.gray);
    expect(getStateColor('ANY_STATE')).toBe(COLOR.gray);
  });
});
