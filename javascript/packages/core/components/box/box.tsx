import { getOverrides } from 'baseui';

import { DescriptionText } from '#core/components/description-text';
import { StyledBoxContainer, StyledBoxHeader, StyledBoxTitle } from './styled-components';

import type { BoxProps } from './types';

/**
 * Container component that groups related content with an optional title and description.
 *
 * Box provides a consistent way to organize and visually group content sections throughout
 * the application. It includes support for customizable styling through BaseUI overrides.
 *
 * Features:
 * - Optional title and description header
 * - Consistent spacing and borders
 * - Customizable through BaseUI overrides system
 * - Integrates with application theme
 *
 * @param props.title - Optional title displayed at the top of the box
 * @param props.description - Optional description text shown below the title
 * @param props.children - Content to display within the box
 * @param props.overrides - BaseUI overrides for BoxContainer, BoxHeader, and BoxTitle components
 *
 * @example
 * ```tsx
 * // Simple box with title
 * <Box title="Pipeline Configuration">
 *   <PipelineForm />
 * </Box>
 *
 * // With title and description
 * <Box
 *   title="Advanced Settings"
 *   description="Configure advanced pipeline options"
 * >
 *   <AdvancedSettingsForm />
 * </Box>
 *
 * // Without header (content only)
 * <Box>
 *   <MetricsChart />
 * </Box>
 *
 * // With custom styling
 * <Box
 *   title="Custom Styled Box"
 *   overrides={{
 *     BoxContainer: {
 *       style: { backgroundColor: '#f5f5f5' }
 *     }
 *   }}
 * >
 *   <CustomContent />
 * </Box>
 * ```
 */
export function Box(props: BoxProps) {
  const { children, description, overrides = {}, title } = props;

  const [BoxContainer, boxContainerProps] = getOverrides(
    overrides.BoxContainer,
    StyledBoxContainer
  );
  const [BoxHeader, boxHeaderProps] = getOverrides(overrides.BoxHeader, StyledBoxHeader);
  const [BoxTitle, boxTitleProps] = getOverrides(overrides.BoxTitle, StyledBoxTitle);

  return (
    <BoxContainer {...boxContainerProps}>
      {title && (
        <BoxHeader {...boxHeaderProps}>
          <BoxTitle {...boxTitleProps}>{title}</BoxTitle>
          <DescriptionText>{description}</DescriptionText>
        </BoxHeader>
      )}
      {children}
    </BoxContainer>
  );
}
