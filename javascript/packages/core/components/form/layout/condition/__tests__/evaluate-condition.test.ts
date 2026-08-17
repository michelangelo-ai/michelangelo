import { evaluateCondition } from '#core/components/form/layout/condition/evaluate-condition';

import type { ConditionLayoutConfig } from '#core/components/form/layout/condition/types';

describe('evaluateCondition', () => {
  describe('is', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'mode',
      is: 'advanced',
      items: [],
    };

    it('returns true when the value matches', () => {
      expect(evaluateCondition(layout, 'advanced')).toBe(true);
    });

    it('returns false when the value does not match', () => {
      expect(evaluateCondition(layout, 'basic')).toBe(false);
    });
  });

  describe('isNot', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'mode',
      isNot: 'hidden',
      items: [],
    };

    it('returns false when the value equals isNot', () => {
      expect(evaluateCondition(layout, 'hidden')).toBe(false);
    });

    it('returns true when the value differs from isNot and is non-empty', () => {
      expect(evaluateCondition(layout, 'shown')).toBe(true);
    });

    it('returns false when the value is empty', () => {
      expect(evaluateCondition(layout, '')).toBe(false);
    });
  });

  describe('isEmpty: true', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'name',
      isEmpty: true,
      items: [],
    };

    it('returns true when the value is empty', () => {
      expect(evaluateCondition(layout, '')).toBe(true);
    });

    it('returns false when the value is non-empty', () => {
      expect(evaluateCondition(layout, 'Alice')).toBe(false);
    });
  });

  describe('isEmpty: false', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'name',
      isEmpty: false,
      items: [],
    };

    it('returns true when the value is non-empty', () => {
      expect(evaluateCondition(layout, 'Alice')).toBe(true);
    });

    it('returns false when the value is empty', () => {
      expect(evaluateCondition(layout, '')).toBe(false);
    });
  });

  describe('containsAny', () => {
    const layout: ConditionLayoutConfig = {
      type: 'condition',
      when: 'role',
      containsAny: ['admin', 'superadmin'],
      items: [],
    };

    it('returns true when a scalar value is in the list', () => {
      expect(evaluateCondition(layout, 'admin')).toBe(true);
    });

    it('returns false when a scalar value is not in the list', () => {
      expect(evaluateCondition(layout, 'guest')).toBe(false);
    });

    it('returns true when an array value overlaps with the list', () => {
      expect(evaluateCondition(layout, ['guest', 'admin'])).toBe(true);
    });

    it('returns false when an array value does not overlap with the list', () => {
      expect(evaluateCondition(layout, ['guest', 'viewer'])).toBe(false);
    });
  });
});
