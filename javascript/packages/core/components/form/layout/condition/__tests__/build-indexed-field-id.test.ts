import { buildIndexedFieldId } from '#core/components/form/layout/condition/build-indexed-field-id';

describe('buildIndexedFieldId', () => {
  it('inserts the index after the root path and preserves the suffix', () => {
    expect(
      buildIndexedFieldId({
        entityId: 'spec.messages.contents.text',
        rootFieldPath: 'spec.messages[3].contents',
        index: 1,
      })
    ).toBe('spec.messages[3].contents[1].text');
  });

  it('produces no suffix when entityId matches the root path exactly', () => {
    expect(
      buildIndexedFieldId({
        entityId: 'spec.messages.contents',
        rootFieldPath: 'spec.messages[3].contents',
        index: 2,
      })
    ).toBe('spec.messages[3].contents[2]');
  });

  it('strips multiple existing indices from a nested repeated root path', () => {
    expect(
      buildIndexedFieldId({
        entityId: 'spec.messages.items.nested.text',
        rootFieldPath: 'spec.messages[3].items[0].nested',
        index: 5,
      })
    ).toBe('spec.messages[3].items[0].nested[5].text');
  });

  it('supports index 0', () => {
    expect(
      buildIndexedFieldId({
        entityId: 'spec.messages.contents.text',
        rootFieldPath: 'spec.messages[3].contents',
        index: 0,
      })
    ).toBe('spec.messages[3].contents[0].text');
  });
});
