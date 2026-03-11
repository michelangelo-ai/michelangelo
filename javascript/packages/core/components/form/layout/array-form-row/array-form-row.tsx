import React from 'react';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';

import { useArrayField } from '#core/components/form/hooks/use-array-field';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { RepeatedLayoutProvider } from '#core/providers/repeated-layout-provider/repeated-layout-provider';

import type { ArrayFormRowProps } from './types';

export function ArrayFormRow({
  rootFieldPath,
  minItems = 0,
  readOnly = false,
  children,
  name,
  span,
}: ArrayFormRowProps) {
  const [css, theme] = useStyletron();
  const { entries, add, remove, isRemovable } = useArrayField(rootFieldPath, {
    minItems,
    readOnly,
  });

  return (
    <>
      {entries.map(({ id, indexedFieldPath }, index) => {
        const rowContent = children(indexedFieldPath, index);
        // Unwrap a top-level Fragment so callers can return multiple fields
        // from the render prop and have each placed in its own grid column.
        const rowChildren =
          React.isValidElement<React.PropsWithChildren>(rowContent) &&
          rowContent.type === React.Fragment
            ? rowContent.props.children
            : rowContent;

        return (
          <RepeatedLayoutProvider key={id} index={index} rootFieldPath={rootFieldPath}>
            <div
              className={css({ display: 'flex', alignItems: 'end', gap: theme.sizing.scale300 })}
            >
              <FormRow name={name} span={span}>
                {rowChildren}
              </FormRow>
              {isRemovable && (
                <Button
                  type="button"
                  kind={KIND.tertiary}
                  shape={SHAPE.circle}
                  size={SIZE.default}
                  aria-label="Remove"
                  onClick={() => remove(index)}
                >
                  <Icon name="deleteAlt" kind={IconKind.SECONDARY} />
                </Button>
              )}
            </div>
          </RepeatedLayoutProvider>
        );
      })}
      {!readOnly && (
        <Button
          type="button"
          kind={KIND.secondary}
          size={SIZE.compact}
          startEnhancer={
            <Icon name="plus" color={theme.colors.contentPrimary} size={theme.sizing.scale600} />
          }
          overrides={{
            BaseButton: { style: { marginBottom: theme.sizing.scale600, width: '260px' } },
          }}
          onClick={add}
        >
          Add more
        </Button>
      )}
    </>
  );
}
