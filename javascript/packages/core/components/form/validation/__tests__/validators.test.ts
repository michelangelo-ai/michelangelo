import { describe, expect, it } from 'vitest';

import { combineValidators } from '#core/components/form/validation/combine-validators';
import {
  max,
  maxLength,
  min,
  minLength,
  regex,
  required,
  url,
} from '#core/components/form/validation/validators';

describe('required', () => {
  it('returns undefined for non-empty string', () => {
    expect(required()('hello')).toBeUndefined();
  });

  it('returns error for empty string', () => {
    expect(required()('')).toBe('This field is required.');
  });

  it('returns error for whitespace-only string', () => {
    expect(required()('   ')).toBe('This field is required.');
  });

  it('returns error for null', () => {
    expect(required()(null)).toBe('This field is required.');
  });

  it('returns error for undefined', () => {
    expect(required()(undefined)).toBe('This field is required.');
  });

  it('returns error for empty array', () => {
    expect(required()([])).toBe('This field is required.');
  });

  it('returns error for empty object', () => {
    expect(required()({})).toBe('This field is required.');
  });

  it('returns undefined for non-empty array', () => {
    expect(required()(['a'])).toBeUndefined();
  });

  it('returns undefined for boolean false', () => {
    expect(required()(false)).toBeUndefined();
  });

  it('returns undefined for number 0', () => {
    expect(required()(0)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(required('Required!')('')).toBe('Required!');
  });
});

describe('min', () => {
  it('returns undefined for value above minimum', () => {
    expect(min(5)(10)).toBeUndefined();
  });

  it('returns undefined for value equal to minimum', () => {
    expect(min(5)(5)).toBeUndefined();
  });

  it('returns error for value below minimum', () => {
    expect(min(5)(3)).toBe('Must be at least 5.');
  });

  it('returns undefined for empty value', () => {
    expect(min(5)('')).toBeUndefined();
    expect(min(5)(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(min(5, 'Too small')(3)).toBe('Too small');
  });
});

describe('max', () => {
  it('returns undefined for value below maximum', () => {
    expect(max(10)(5)).toBeUndefined();
  });

  it('returns undefined for value equal to maximum', () => {
    expect(max(10)(10)).toBeUndefined();
  });

  it('returns error for value above maximum', () => {
    expect(max(10)(15)).toBe('Must be at most 10.');
  });

  it('returns undefined for empty value', () => {
    expect(max(10)('')).toBeUndefined();
    expect(max(10)(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(max(10, 'Too large')(15)).toBe('Too large');
  });
});

describe('minLength', () => {
  it('returns undefined for string meeting minimum length', () => {
    expect(minLength(3)('abc')).toBeUndefined();
  });

  it('returns undefined for string longer than minimum', () => {
    expect(minLength(3)('abcd')).toBeUndefined();
  });

  it('returns error for string shorter than minimum', () => {
    expect(minLength(3)('ab')).toBe('Must be at least 3 characters.');
  });

  it('returns undefined for empty value', () => {
    expect(minLength(3)('')).toBeUndefined();
    expect(minLength(3)(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(minLength(3, 'Too short')('ab')).toBe('Too short');
  });
});

describe('maxLength', () => {
  it('returns undefined for string within maximum length', () => {
    expect(maxLength(5)('abc')).toBeUndefined();
  });

  it('returns undefined for string equal to maximum length', () => {
    expect(maxLength(5)('abcde')).toBeUndefined();
  });

  it('returns error for string exceeding maximum length', () => {
    expect(maxLength(5)('abcdef')).toBe('Must be at most 5 characters.');
  });

  it('returns undefined for empty value', () => {
    expect(maxLength(5)('')).toBeUndefined();
    expect(maxLength(5)(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(maxLength(5, 'Too long')('abcdef')).toBe('Too long');
  });
});

describe('regex', () => {
  it('returns undefined for string matching pattern', () => {
    expect(regex(/^[a-z]+$/)('hello')).toBeUndefined();
  });

  it('returns error for string not matching pattern', () => {
    expect(regex(/^[a-z]+$/)('Hello123')).toBe('Invalid format.');
  });

  it('accepts string pattern', () => {
    expect(regex('^[0-9]+$')('123')).toBeUndefined();
    expect(regex('^[0-9]+$')('abc')).toBe('Invalid format.');
  });

  it('returns undefined for empty value', () => {
    expect(regex(/^[a-z]+$/)('')).toBeUndefined();
    expect(regex(/^[a-z]+$/)(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(regex(/^[a-z]+$/, 'Letters only')('123')).toBe('Letters only');
  });
});

describe('url', () => {
  it('returns undefined for valid absolute URL', () => {
    expect(url()('https://example.com')).toBeUndefined();
  });

  it('returns error for relative URL', () => {
    expect(url()('/relative/path')).toBe('Must be a valid URL.');
  });

  it('returns error for invalid URL', () => {
    expect(url()('not a url')).toBe('Must be a valid URL.');
  });

  it('returns undefined for empty value', () => {
    expect(url()('')).toBeUndefined();
    expect(url()(undefined)).toBeUndefined();
  });

  it('uses custom error message', () => {
    expect(url('Enter a valid URL')('bad')).toBe('Enter a valid URL');
  });
});

describe('combineValidators', () => {
  it('returns undefined when all validators pass', () => {
    expect(combineValidators(required(), minLength(2))('hello')).toBeUndefined();
  });

  it('returns first error when first validator fails', () => {
    expect(combineValidators(required(), minLength(2))('')).toBe('This field is required.');
  });

  it('returns second error when only second validator fails', () => {
    expect(combineValidators(required(), minLength(10))('hi')).toBe(
      'Must be at least 10 characters.'
    );
  });

  it('stops at first error and does not run subsequent validators', () => {
    const neverCalled = (value: unknown) => {
      throw new Error(`Should not be called with: ${String(value)}`);
    };
    expect(combineValidators(required(), neverCalled)('')).toBe('This field is required.');
  });

  it('returns undefined when no validators are provided', () => {
    expect(combineValidators()('anything')).toBeUndefined();
  });
});
