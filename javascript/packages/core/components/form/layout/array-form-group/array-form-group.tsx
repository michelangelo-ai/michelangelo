import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

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
  const [, theme] = useStyletron();
  const { entries, add, remove, isRemovable } = useArrayField(rootFieldPath, {
    minItems,
    readOnly,
  });
  const addLabel = addLabelProp ?? (groupLabel ? `Add ${groupLabel.toLowerCase()}` : 'Add more');

  return (
    <>
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
              // FormGroup comes with marginBottom that intends to be applied to separate groups in the
              // context of an entire form. Since the ArrayFormGroup has a button at the end, we need to
              // remove the margin bottom to avoid disconnect between the form group and the button.
              BoxContainer: { style: { marginBottom: 0 } },
            }}
          >
            {children(indexedFieldPath, index)}
          </FormGroup>
        </RepeatedLayoutProvider>
      ))}
      {!readOnly && (
        <Button
          type="button"
          kind={KIND.secondary}
          shape={SHAPE.pill}
          size={SIZE.compact}
          startEnhancer={
            <Icon name="plus" color={theme.colors.contentPrimary} size={theme.sizing.scale600} />
          }
          overrides={{
            BaseButton: { style: { marginBottom: theme.sizing.scale600, width: '260px' } },
          }}
          onClick={add}
        >
          {addLabel}
        </Button>
      )}
    </>
  );
}
