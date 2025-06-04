import '@testing-library/jest-dom';
import { vi } from 'vitest';

declare global {
  var vi: (typeof import('vitest'))['vi'];
}

global.vi = vi;
