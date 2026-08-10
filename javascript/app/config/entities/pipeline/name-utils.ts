const SUFFIX_DELIMITER = '-';
const UUID_SUFFIX_LENGTH = 8;

function parseIsoString(isoString: string): { date: string; time: string } | null {
  if (isNaN(Date.parse(isoString))) {
    return null;
  }

  const parts = isoString.split('T');
  if (parts.length !== 2) {
    return null;
  }

  return { date: parts[0], time: parts[1] };
}

export const generateSuffix = (config: { withDate: boolean } = { withDate: false }): string => {
  const uuidSuffix = `${SUFFIX_DELIMITER}${crypto.randomUUID().substring(0, UUID_SUFFIX_LENGTH)}`;

  if (config.withDate) {
    const isoString = new Date().toISOString();
    const parsed = parseIsoString(isoString);

    if (!parsed) {
      console.warn('Date.toISOString() returned an invalid ISO string', isoString);
      return uuidSuffix;
    }

    const { date, time } = parsed;
    const compactDate = date.replace(/-/g, '');
    const compactTime = time.replace(/\..*$/, '').replace(/:/g, '');

    return `${SUFFIX_DELIMITER}${compactDate}-${compactTime}${uuidSuffix}`;
  }

  return uuidSuffix;
};
