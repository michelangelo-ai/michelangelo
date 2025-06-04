import { forwardRef } from 'react';
import { mergeOverrides, useStyletron } from 'baseui';
import { KIND as BASE_KIND, Tag as BaseTag } from 'baseui/tag';

import { BEHAVIOR, COLOR, HIERARCHY, SIZE } from './constants';
import { getTagOverrides } from './styled-components';

import type { TagKind as BaseTagKind, TagSize as BaseTagSize } from 'baseui/tag';
import type { Props } from './types';

export const Tag = forwardRef<HTMLElement, Props>(
  (
    {
      children,
      overrides,
      size = SIZE.small,
      color = COLOR.gray,
      behavior = BEHAVIOR.display,
      hierarchy = HIERARCHY.secondary,
      ...rest
    },
    ref
  ) => {
    const [_, theme] = useStyletron();

    const baseSize: BaseTagSize | undefined = size === SIZE.xSmall ? SIZE.small : size;
    const baseKind: BaseTagKind | undefined =
      color === COLOR.gray || color === COLOR.purple || color === COLOR.magenta
        ? BASE_KIND.black
        : color;

    return (
      <BaseTag
        {...rest}
        ref={ref}
        kind={baseKind}
        size={baseSize}
        hierarchy={hierarchy}
        overrides={mergeOverrides(
          overrides,
          getTagOverrides(theme, {
            size,
            color,
            behavior,
            hierarchy,
          })
        )}
      >
        {children}
      </BaseTag>
    );
  }
);

Tag.displayName = 'Tag';
