'use strict';

const fs = require('node:fs');
const path = require('node:path');

const SANDBOX_GATEWAY = 'https://openapi-sandbox.dl.alipaydev.com/gateway.do';
const PRODUCTION_GATEWAY = 'https://openapi.alipay.com/gateway.do';

class PaymentConfigError extends Error {
  constructor(message) {
    super(message);
    this.name = 'PaymentConfigError';
  }
}

function requiredString(value, field) {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new PaymentConfigError(`支付宝配置缺少 ${field}`);
  }
  return value.trim();
}

function optionalUrl(value, field, { httpsOnly = false } = {}) {
  if (typeof value !== 'string' || value.trim() === '') return '';
  let parsed;
  try {
    parsed = new URL(value.trim());
  } catch {
    throw new PaymentConfigError(`${field} 不是有效 URL`);
  }
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new PaymentConfigError(`${field} 只支持 HTTP/HTTPS`);
  }
  if (httpsOnly && parsed.protocol !== 'https:') {
    throw new PaymentConfigError(`${field} 必须使用 HTTPS`);
  }
  return parsed.toString().replace(/\/$/, '');
}

function loadSandboxConfig(env, fsImpl) {
  const projectRoot = env.ALIPAY_COFFEE_PROJECT_ROOT || path.resolve(__dirname, '../../..');
  const configPath = path.resolve(
    env.ALIPAY_CONFIG_PATH || path.join(projectRoot, '.alipay-sandbox.json')
  );

  let fileInfo;
  try {
    fileInfo = fsImpl.lstatSync(configPath);
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      throw new PaymentConfigError('支付宝沙箱配置尚未创建');
    }
    throw new PaymentConfigError('无法检查支付宝沙箱配置');
  }
  if (!fileInfo.isFile() || fileInfo.isSymbolicLink()) {
    throw new PaymentConfigError('支付宝沙箱配置必须是安全的普通文件');
  }
  if (process.platform !== 'win32' && (fileInfo.mode & 0o777) !== 0o600) {
    throw new PaymentConfigError('支付宝沙箱配置权限必须为 0600');
  }

  let raw;
  try {
    raw = fsImpl.readFileSync(configPath, 'utf8');
  } catch (error) {
    throw new PaymentConfigError('无法读取支付宝沙箱配置');
  }

  let data;
  try {
    data = JSON.parse(raw);
  } catch {
    throw new PaymentConfigError('支付宝沙箱配置不是有效 JSON');
  }

  const app = Array.isArray(data.appIds) ? data.appIds[0] : null;
  if (!app || typeof app !== 'object') {
    throw new PaymentConfigError('支付宝沙箱配置缺少 appIds[0]');
  }

  const sellerId =
    (typeof app.pid === 'string' && app.pid.trim()) ||
    (data.sandboxAccounts &&
      data.sandboxAccounts.partner &&
      typeof data.sandboxAccounts.partner.userId === 'string' &&
      data.sandboxAccounts.partner.userId.trim()) ||
    '';

  return {
    mode: 'sandbox',
    configPath,
    appId: requiredString(app.appId, 'appIds[0].appId'),
    privateKey: requiredString(app.appPrivatePkcsKey, 'appIds[0].appPrivatePkcsKey'),
    alipayPublicKey: requiredString(app.alipayPublicKey, 'appIds[0].alipayPublicKey'),
    sellerId: requiredString(sellerId, '沙箱商家 userId/pid'),
    sellerEmail: '',
    gateway: SANDBOX_GATEWAY,
    notifyUrl: optionalUrl(env.ALIPAY_NOTIFY_URL, 'ALIPAY_NOTIFY_URL', { httpsOnly: true }),
    publicOrigin: optionalUrl(env.ALIPAY_PUBLIC_ORIGIN, 'ALIPAY_PUBLIC_ORIGIN'),
  };
}

function loadProductionConfig(env) {
  const sellerId = typeof env.ALIPAY_SELLER_ID === 'string' ? env.ALIPAY_SELLER_ID.trim() : '';
  const sellerEmail =
    typeof env.ALIPAY_SELLER_EMAIL === 'string' ? env.ALIPAY_SELLER_EMAIL.trim() : '';
  if (!sellerId && !sellerEmail) {
    throw new PaymentConfigError('生产配置需要 ALIPAY_SELLER_ID 或 ALIPAY_SELLER_EMAIL');
  }

  return {
    mode: 'production',
    configPath: '',
    appId: requiredString(env.ALIPAY_APP_ID, 'ALIPAY_APP_ID'),
    privateKey: requiredString(env.ALIPAY_PRIVATE_KEY, 'ALIPAY_PRIVATE_KEY'),
    alipayPublicKey: requiredString(env.ALIPAY_PUBLIC_KEY, 'ALIPAY_PUBLIC_KEY'),
    sellerId,
    sellerEmail,
    gateway: PRODUCTION_GATEWAY,
    notifyUrl: optionalUrl(env.ALIPAY_NOTIFY_URL, 'ALIPAY_NOTIFY_URL', { httpsOnly: true }),
    publicOrigin: optionalUrl(env.ALIPAY_PUBLIC_ORIGIN, 'ALIPAY_PUBLIC_ORIGIN', {
      httpsOnly: true,
    }),
  };
}

function loadPaymentConfig(env = process.env, fsImpl = fs) {
  const mode = String(env.ALIPAY_ENV || 'sandbox').trim().toLowerCase();
  if (mode === 'production') return loadProductionConfig(env);
  if (mode !== 'sandbox') {
    throw new PaymentConfigError('ALIPAY_ENV 只支持 sandbox 或 production');
  }
  return loadSandboxConfig(env, fsImpl);
}

function paymentConfigStatus(configProvider = loadPaymentConfig) {
  try {
    const config = configProvider();
    return {
      ready: true,
      mode: config.mode,
      notifyEnabled: Boolean(config.notifyUrl),
    };
  } catch (error) {
    if (!(error instanceof PaymentConfigError)) throw error;
    return {
      ready: false,
      mode: String(process.env.ALIPAY_ENV || 'sandbox').toLowerCase(),
      notifyEnabled: false,
      message: error.message,
    };
  }
}

module.exports = {
  PaymentConfigError,
  SANDBOX_GATEWAY,
  PRODUCTION_GATEWAY,
  loadPaymentConfig,
  paymentConfigStatus,
};
