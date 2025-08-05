describe('Test Environment Configuration', () => {
  it('should parse dates consistently across environments', () => {
    const isoString = '2023-06-15T14:30:00.000Z';
    const parsedDate = new Date(isoString);

    // In UTC environment, local time should equal UTC time
    expect(parsedDate.getFullYear()).toBe(parsedDate.getUTCFullYear());
    expect(parsedDate.getHours()).toBe(parsedDate.getUTCHours());
    expect(parsedDate.getMonth()).toBe(parsedDate.getUTCMonth());
    expect(parsedDate.getDate()).toBe(parsedDate.getUTCDate());
  });

  it('should handle midnight boundaries without DST issues', () => {
    // Test dates around common DST transition periods
    const springForward = new Date('2023-03-12T06:00:00.000Z'); // Common DST start
    const fallBack = new Date('2023-11-05T06:00:00.000Z'); // Common DST end

    // In UTC, these should behave predictably
    expect(springForward.getUTCHours()).toBe(6);
    expect(fallBack.getUTCHours()).toBe(6);

    // Local and UTC should be identical in test environment
    expect(springForward.getHours()).toBe(springForward.getUTCHours());
    expect(fallBack.getHours()).toBe(fallBack.getUTCHours());
  });
});
