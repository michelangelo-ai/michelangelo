import { filterHiddenConditionFields } from '#core/components/form/utils/filter-hidden-condition-fields';

import type { FormConfig } from '#core/components/form/types/config-types';

describe('filterHiddenConditionFields', () => {
  it('returns values unchanged when the layout has no conditions', () => {
    const config: FormConfig = { fields: {}, layout: ['name', 'email'] };
    const values = { name: 'Alice', email: 'alice@example.com' };

    expect(filterHiddenConditionFields(values, config)).toEqual(values);
  });

  it('keeps a field whose is condition currently evaluates to true', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        { type: 'condition', when: 'mode', is: 'advanced', items: ['advancedSetting'] },
      ],
    };
    const values = { mode: 'advanced', advancedSetting: 'value' };

    expect(filterHiddenConditionFields(values, config)).toEqual(values);
  });

  it('strips a field whose is condition currently evaluates to false', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        { type: 'condition', when: 'mode', is: 'advanced', items: ['advancedSetting'] },
      ],
    };
    const values = { mode: 'basic', advancedSetting: 'stale' };

    expect(filterHiddenConditionFields(values, config)).toEqual({ mode: 'basic' });
  });

  it('strips every leaf field nested under a false is condition wrapping a group', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        {
          type: 'condition',
          when: 'mode',
          is: 'advanced',
          items: [{ type: 'group', title: 'Advanced', items: ['a', 'b'] }],
        },
      ],
    };
    const values = { mode: 'basic', a: 'stale-a', b: 'stale-b' };

    expect(filterHiddenConditionFields(values, config)).toEqual({ mode: 'basic' });
  });

  it('strips a nested is condition when the outer is condition is false, regardless of the inner condition', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        'role',
        {
          type: 'condition',
          when: 'mode',
          is: 'advanced',
          items: [{ type: 'condition', when: 'role', is: 'admin', items: ['adminPanel'] }],
        },
      ],
    };
    const values = { mode: 'basic', role: 'admin', adminPanel: 'stale' };

    expect(filterHiddenConditionFields(values, config)).toEqual({ mode: 'basic', role: 'admin' });
  });

  it('strips only the false nested is condition when the outer is condition is true', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        'role',
        'outerField',
        {
          type: 'condition',
          when: 'mode',
          is: 'advanced',
          items: [
            'outerField',
            { type: 'condition', when: 'role', is: 'admin', items: ['adminPanel'] },
          ],
        },
      ],
    };
    const values = { mode: 'advanced', role: 'guest', outerField: 'kept', adminPanel: 'stale' };

    expect(filterHiddenConditionFields(values, config)).toEqual({
      mode: 'advanced',
      role: 'guest',
      outerField: 'kept',
    });
  });

  it('only strips the false condition among multiple sibling is conditions', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        'role',
        { type: 'condition', when: 'mode', is: 'advanced', items: ['advancedSetting'] },
        { type: 'condition', when: 'role', is: 'admin', items: ['adminPanel'] },
      ],
    };
    const values = {
      mode: 'advanced',
      role: 'guest',
      advancedSetting: 'kept',
      adminPanel: 'stale',
    };

    expect(filterHiddenConditionFields(values, config)).toEqual({
      mode: 'advanced',
      role: 'guest',
      advancedSetting: 'kept',
    });
  });

  it('never touches fields not wrapped in any condition', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'name',
        { type: 'condition', when: 'mode', is: 'advanced', items: ['advancedSetting'] },
      ],
    };
    const values = { name: 'Alice', mode: 'basic', advancedSetting: 'stale' };

    expect(filterHiddenConditionFields(values, config)).toEqual({ name: 'Alice', mode: 'basic' });
  });

  it('does not strip fields under a non-is condition (not yet supported)', () => {
    const config: FormConfig = {
      fields: {},
      layout: [
        'mode',
        { type: 'condition', when: 'mode', isNot: 'advanced', items: ['basicSetting'] },
      ],
    };
    const values = { mode: 'advanced', basicSetting: 'still present' };

    expect(filterHiddenConditionFields(values, config)).toEqual(values);
  });
});
