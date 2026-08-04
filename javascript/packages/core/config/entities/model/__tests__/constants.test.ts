import { MODEL_KIND, MODEL_KIND_TEXT_MAP } from '../constants';

describe('MODEL_KIND_TEXT_MAP', () => {
  test.each(Object.entries(MODEL_KIND))('maps MODEL_KIND.%s to a display label', (_name, value) => {
    expect(MODEL_KIND_TEXT_MAP[value]).toEqual(expect.any(String));
  });

  test("keys are numeric, matching the proto client's decoded enum shape", () => {
    expect(MODEL_KIND_TEXT_MAP[MODEL_KIND.REGRESSION]).toEqual('Regression');
    expect(MODEL_KIND_TEXT_MAP[MODEL_KIND.BINARY_CLASSIFICATION]).toEqual('Binary Classification');
    expect(MODEL_KIND_TEXT_MAP[MODEL_KIND.CLUSTERING]).toEqual('Clustering');
  });
});
