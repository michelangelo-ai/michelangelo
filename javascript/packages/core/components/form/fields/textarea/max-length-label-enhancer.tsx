import { StyledMaxLengthContainer } from './styled-components';

import type { MaxLengthLabelEnhancerProps } from './types';

export const MaxLengthLabelEnhancer: React.FC<MaxLengthLabelEnhancerProps> = ({
  maxLength,
  currentLength,
}) => {
  return (
    <StyledMaxLengthContainer>
      {currentLength} / {maxLength}
    </StyledMaxLengthContainer>
  );
};
