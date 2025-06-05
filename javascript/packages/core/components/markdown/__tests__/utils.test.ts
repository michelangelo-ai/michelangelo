import { formatMarkdownText } from '../utils';

describe('markdown utils', () => {
  describe('formatMarkdownText', () => {
    it('should escape underscores between letters', () => {
      expect(formatMarkdownText('PIPELINE_MANIFEST_TYPE_INVALID')).toBe(
        'PIPELINE\\_MANIFEST\\_TYPE\\_INVALID'
      );
      expect(formatMarkdownText('metadata.name')).toBe('metadata.name');
      expect(formatMarkdownText('metadata_namespace')).toBe('metadata\\_namespace');
    });

    it('should not escape underscores at the start or end of text', () => {
      expect(formatMarkdownText('_italic_')).toBe('_italic_');
      expect(formatMarkdownText('_start')).toBe('_start');
      expect(formatMarkdownText('end_')).toBe('end_');
    });

    it('should not escape underscores between non-letter characters', () => {
      expect(formatMarkdownText('123_456')).toBe('123_456');
      expect(formatMarkdownText('!@#$%_^&*()')).toBe('!@#$%_^&*()');
    });

    it('should normalize long sequences of spaces', () => {
      const longSpaces = ' '.repeat(60);
      const result = formatMarkdownText(`text${longSpaces}text`);
      expect(result).toBe('text' + '·'.repeat(60) + 'text');
    });

    it('should handle empty strings', () => {
      expect(formatMarkdownText('')).toBe('');
    });

    it('should handle strings without underscores or long spaces', () => {
      expect(formatMarkdownText('normal text')).toBe('normal text');
    });
  });
});
