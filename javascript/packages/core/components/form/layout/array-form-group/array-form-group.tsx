import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

import { AddButton } from '#core/components/form/components/add-button/add-button';
import { useArrayField } from '#core/components/form/hooks/use-array-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { RepeatedLayoutProvider } from '#core/providers/repeated-layout-provider/repeated-layout-provider';

import type { ArrayFormGroupProps } from './types';

export function ArrayFormGroup({
  rootFieldPath,
  groupLabel,
  addLabel: addLabelProp,
  minItems = 0,
  readOnly = false,
  children,
  description,
  tooltip,
  collapsible,
}: ArrayFormGroupProps) {
  const { entries, handleItemAdd, remove, isRemovable } = useArrayField(rootFieldPath, {
    minItems,
    readOnly,
  });
  const addLabel = addLabelProp ?? (groupLabel ? `Add ${groupLabel.toLowerCase()}` : 'Add more');
  const [css, theme] = useStyletron();

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale800 })}>
      {entries.map(({ id, indexedFieldPath }, index) => (
        <RepeatedLayoutProvider key={id} index={index} rootFieldPath={rootFieldPath}>
          <FormGroup
            title={groupLabel ? `${groupLabel} ${index + 1}` : undefined}
            description={description}
            tooltip={tooltip}
            collapsible={collapsible}
            endEnhancer={
              isRemovable && (
                <Button
                  type="button"
                  kind={KIND.secondary}
                  shape={SHAPE.pill}
                  size={SIZE.compact}
                  startEnhancer={<Icon name="trashCan" kind={IconKind.PRIMARY} />}
                  aria-label="Remove"
                  onClick={() => remove(index)}
                >
                  Remove
                </Button>
              )
            }
            overrides={{
              // ArrayFormGroup owns spacing between its own entries and the trailing AddButton via
              // the wrapping flex gap above, so each entry's own marginBottom would double up
              // (flex gap and margin are additive, not collapsing) and needs to be zeroed here.
              BoxContainer: { style: { marginBottom: 0 } },
            }}
          >
            {children(indexedFieldPath, index)}
          </FormGroup>
        </RepeatedLayoutProvider>
      ))}
      {!readOnly && <AddButton label={addLabel} shape={SHAPE.pill} onClick={handleItemAdd} />}
    </div>
  );
}
