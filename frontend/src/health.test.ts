import { expect, test } from 'vitest';

import { healthStatus } from './health';

test('healthStatus returns ok', () => {
  expect(healthStatus().status).toBe('ok');
});
