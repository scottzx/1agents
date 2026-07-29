'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { afterEach, test } = require('node:test');
const { PaymentConfigError, loadPaymentConfig } = require('../src/config');

const tempDirs = [];

afterEach(async () => {
  await Promise.all(tempDirs.splice(0).map(dir => fsp.rm(dir, { recursive: true, force: true })));
});

async function sandboxFile(mode = 0o600) {
  const dir = await fsp.mkdtemp(path.join(os.tmpdir(), 'alipay-coffee-config-'));
  tempDirs.push(dir);
  const file = path.join(dir, '.alipay-sandbox.json');
  await fsp.writeFile(
    file,
    JSON.stringify({
      appIds: [
        {
          appId: 'sandbox-app',
          appPrivatePkcsKey: 'pkcs1-private-key',
          alipayPublicKey: 'alipay-public-key',
          pid: '2088000000000000',
        },
      ],
      sandboxAccounts: {
        partner: { userId: '2088000000000000' },
      },
    }),
    { mode }
  );
  await fsp.chmod(file, mode);
  return file;
}

test('maps the verified appIds[0] sandbox fields for Node.js', async () => {
  const configPath = await sandboxFile();
  const config = loadPaymentConfig(
    {
      ALIPAY_ENV: 'sandbox',
      ALIPAY_CONFIG_PATH: configPath,
    },
    fs
  );

  assert.equal(config.mode, 'sandbox');
  assert.equal(config.appId, 'sandbox-app');
  assert.equal(config.privateKey, 'pkcs1-private-key');
  assert.equal(config.sellerId, '2088000000000000');
  assert.equal(config.notifyUrl, '');
});

test('rejects sandbox configuration with permissions wider than 0600', async () => {
  if (process.platform === 'win32') return;
  const configPath = await sandboxFile(0o644);
  assert.throws(
    () =>
      loadPaymentConfig(
        {
          ALIPAY_ENV: 'sandbox',
          ALIPAY_CONFIG_PATH: configPath,
        },
        fs
      ),
    error => error instanceof PaymentConfigError && /0600/.test(error.message)
  );
});
