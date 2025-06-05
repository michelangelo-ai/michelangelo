import { BooleanCell } from './renderers/boolean/boolean-cell';
import { DateCell } from './renderers/date/date-cell';
import { DescriptionCell } from './renderers/description/description-cell';
import { LinkCell } from './renderers/link/link-cell';
import { MultiCell } from './renderers/multi/multi-cell';
import { StateCell } from './renderers/state/state-cell';
import { TagCell } from './renderers/tag/tag-cell';
import { TextCell } from './renderers/text/text-cell';
import { TypeCell } from './renderers/type/type-cell';

import type { CellRenderer } from './types';

export enum CellType {
  /**
   * @description Renders a gray **Tag** with formatted text
   */
  TAG = 'TAG',

  /**
   * @description Renders a **Checkmark** with formatted text
   */
  BOOLEAN = 'BOOLEAN',

  /**
   * @description Renders a formatted **Date**
   * @example `2024/01/09 17:53:49`
   */
  DATE = 'DATE',

  /**
   * @description Renders a text slightly smaller and opaque than the standard text
   */
  DESCRIPTION = 'DESCRIPTION',

  /**
   * @description Renders a **Link** with formatted text \
   * This type is implicitly used when a `url` is provided
   */
  LINK = 'LINK',

  /**
   * @description Renders a **Map** with formatted text lines
   */
  MAP = 'MAP',

  /**
   * @description Renders column items in a vertical list
   */
  MULTI = 'MULTI',

  /**
   * @description Renders a **Multi** with formatted text
   */
  REPEATED_ITEMS = 'REPEATED_ITEMS',

  /**
   * @description Renders a **Tag** with coloring and formatted text \
   * Green for success cases, red for error cases and so on
   *
   * @see {@link src/components/cell/state/translation-maps/get-state-kind.ts}
   */
  STATE = 'STATE',

  /**
   * @description Renders different schemas based on typeMeta.kind \
   * This is an implicit type when either `Revision` or `Draft` is provided
   */
  SWITCH_TYPE_META = 'SWITCH_TYPE_META',

  /**
   * @description Renders a standard non formatted/styled text
   */
  TEXT = 'TEXT',

  /**
   * @description Renders a **Badge** with formatted text `Tier {number}`
   */
  TIER = 'TIER',

  /**
   * @description Renders a **Badge** with formatted text \
   * Sentence cased formatted with stripped type prefix or suffix
   */
  TYPE = 'TYPE',
}

export const CELL_RENDERERS: Record<string, CellRenderer<unknown>> = {
  [CellType.BOOLEAN]: BooleanCell as CellRenderer<boolean>,
  [CellType.DATE]: DateCell as CellRenderer<string>,
  [CellType.DESCRIPTION]: DescriptionCell,
  [CellType.LINK]: LinkCell,
  [CellType.MULTI]: MultiCell,
  [CellType.REPEATED_ITEMS]: MultiCell,
  [CellType.STATE]: StateCell as CellRenderer<string>,
  [CellType.TAG]: TagCell,
  [CellType.TYPE]: TypeCell,
  [CellType.TEXT]: TextCell,
};
