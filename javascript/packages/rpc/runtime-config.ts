interface RuntimeConfig {
  apiBaseUrl: string;
}

// Fetches runtime configuration from /config.json.
export async function getApiConfig(): Promise<string> {
  let response: Response;
  try {
    response = await fetch('/config.json');
  } catch (error) {
    if (error instanceof TypeError && error.message.includes('fetch')) {
      console.error(`Config network error: ${error.message}`);
      throw createConfigError('Check network connectivity.', { cause: error });
    }
    console.error(
      `Config fetch error: ${error instanceof Error ? error.message : 'Unknown error'}`
    );
    throw createConfigError('Check that config.json is properly mounted.', { cause: error });
  }

  if (!response.ok) {
    console.error(`Config fetch failed: ${response.status} ${response.statusText}`);
    throw createConfigError('Check that config.json is properly mounted.', { cause: response });
  }

  let config: RuntimeConfig;
  try {
    config = (await response.json()) as RuntimeConfig;
  } catch (error) {
    console.error(
      `Config JSON parsing failed: ${error instanceof Error ? error.message : 'Invalid JSON'}`
    );
    throw createConfigError('Check that config.json contains valid JSON.', { cause: error });
  }

  if (!config.apiBaseUrl) {
    console.error('Config missing apiBaseUrl field', JSON.stringify(config));
    throw createConfigError('Check that config.json contains apiBaseUrl field.');
  }

  return config.apiBaseUrl;
}

function createConfigError(reason: string, options?: ErrorOptions): Error {
  return new Error(`Failed to load runtime configuration. ${reason}`, options);
}
