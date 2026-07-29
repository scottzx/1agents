'use strict';

const crypto = require('node:crypto');
const os = require('node:os');
const path = require('node:path');
const express = require('express');
const { createAlipayClient } = require('./alipay-client');
const {
  PaymentConfigError,
  loadPaymentConfig,
  paymentConfigStatus,
} = require('./config');
const {
  COFFEE_AMOUNTS,
  normalizeAmount,
  requireCoffeeAmount,
  amountLessThanOrEqual,
} = require('./money');
const { JsonOrderStore } = require('./store');

const ORDER_NUMBER_PATTERN = /^COFFEE[0-9A-F]{20,40}$/;
const REFUND_NUMBER_PATTERN = /^RF[0-9A-F]{20,40}$/;

function newId(prefix) {
  const timestamp = Date.now().toString(16).toUpperCase();
  const random = crypto.randomBytes(10).toString('hex').toUpperCase();
  return `${prefix}${timestamp}${random}`;
}

function publicOrder(order) {
  if (!order) return null;
  return {
    outTradeNo: order.outTradeNo,
    totalAmount: order.totalAmount,
    subject: order.subject,
    status: order.status,
    alipayTradeNo: order.alipayTradeNo || '',
    createdAt: order.createdAt,
    updatedAt: order.updatedAt,
    paidAt: order.paidAt || '',
    refunds: order.refunds || {},
  };
}

function isSuccessfulApiResult(result) {
  return result && result.code === '10000';
}

function isPaidStatus(status) {
  return status === 'TRADE_SUCCESS' || status === 'TRADE_FINISHED';
}

function isPaidNotification(params) {
  return (
    isPaidStatus(params.trade_status) &&
    !params.out_biz_no &&
    !params.gmt_refund &&
    !params.refund_fee
  );
}

function businessFieldsMatch(order, params, config) {
  const expectedAmount = normalizeAmount(order.totalAmount);
  const actualAmount = normalizeAmount(params.total_amount);
  const sellerMatches =
    (config.sellerId && params.seller_id === config.sellerId) ||
    (config.sellerEmail && params.seller_email === config.sellerEmail);
  return (
    params.app_id === config.appId &&
    params.out_trade_no === order.outTradeNo &&
    expectedAmount !== null &&
    expectedAmount === actualAmount &&
    Boolean(sellerMatches)
  );
}

function safeNotifyParams(params) {
  const { sign, ...safe } = params;
  return safe;
}

function resolvePublicOrigin(req, config) {
  if (config.publicOrigin) return config.publicOrigin;
  const forwardedHost = String(req.get('x-forwarded-host') || '').split(',')[0].trim();
  const origin = `${req.protocol}://${forwardedHost || req.get('host')}`;
  const parsed = new URL(origin);
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error('无法确定安全的支付回跳地址');
  }
  return parsed.origin;
}

function createDefaultStore() {
  const dataDir =
    process.env.ALIPAY_COFFEE_DATA_DIR ||
    path.join(os.homedir(), '.1agents', 'alipay-coffee');
  return new JsonOrderStore(path.join(dataDir, 'orders.json'));
}

function createApp(options = {}) {
  const app = express();
  const logger = options.logger || console;
  const configProvider = options.configProvider || loadPaymentConfig;
  const clientFactory = options.clientFactory || createAlipayClient;
  const store = options.store || createDefaultStore();
  const publicDir = path.join(__dirname, '..', 'public');

  app.disable('x-powered-by');
  app.set('trust proxy', 'loopback');
  app.use((req, res, next) => {
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Referrer-Policy', 'no-referrer');
    res.setHeader('X-Frame-Options', 'SAMEORIGIN');
    res.setHeader(
      'Content-Security-Policy',
      [
        "default-src 'self'",
        "script-src 'self'",
        "style-src 'self'",
        "img-src 'self' data:",
        "connect-src 'self'",
        "form-action 'self' https://openapi.alipay.com https://openapi-sandbox.dl.alipaydev.com",
        "frame-ancestors 'self'",
        "base-uri 'none'",
        "object-src 'none'",
      ].join('; ')
    );
    res.setHeader('Cache-Control', 'no-store');
    next();
  });

  app.post(
    '/api/coffee/notify',
    express.urlencoded({ extended: false, limit: '64kb' }),
    async (req, res) => {
      const params = { ...req.body };
      const fail = message => {
        try {
          logger.warn(message, safeNotifyParams(params));
        } catch {
          // Logging must not prevent a deterministic plain-text response.
        }
        return res.type('text/plain').send('fail');
      };

      try {
        const config = configProvider();
        if (params.app_id !== config.appId) return fail('alipay notify app_id mismatch');
        const client = clientFactory(config);
        if (!client.verifyNotification(params)) {
          return fail('alipay notify signature check failed');
        }

        const order = await store.getOrder(params.out_trade_no);
        if (!order || !businessFieldsMatch(order, params, config)) {
          return fail('alipay notify business fields mismatch');
        }
        if (!params.notify_id) return fail('alipay notify missing notify_id');

        const persistedParams = safeNotifyParams(params);
        await store.recordNotificationOnce({
          notifyId: params.notify_id,
          params: persistedParams,
          paid: isPaidNotification(params),
        });
        return res.type('text/plain').send('success');
      } catch (error) {
        return fail(error instanceof Error ? error.message : 'alipay notify failed');
      }
    }
  );

  app.use(express.json({ limit: '32kb' }));

  app.get('/api/coffee/config', (req, res) => {
    const status = paymentConfigStatus(configProvider);
    res.json({
      ...status,
      amounts: COFFEE_AMOUNTS,
      subject: '请作者喝杯咖啡',
    });
  });

  app.get('/api/coffee/health', (req, res) => {
    const status = paymentConfigStatus(configProvider);
    res.status(status.ready ? 200 : 503).json({ service: 'alipay-coffee', ...status });
  });

  app.post('/api/coffee/orders', async (req, res, next) => {
    try {
      const totalAmount = requireCoffeeAmount(req.body && req.body.amount);
      const config = configProvider();
      const now = new Date().toISOString();
      const order = {
        outTradeNo: newId('COFFEE'),
        totalAmount,
        subject: '请作者喝杯咖啡',
        status: 'CREATED',
        alipayTradeNo: '',
        refunds: {},
        createdAt: now,
        updatedAt: now,
      };
      await store.createOrder(order);

      const returnUrl = `${resolvePublicOrigin(req, config)}/coffee/return?out_trade_no=${encodeURIComponent(
        order.outTradeNo
      )}`;
      const paymentHtml = clientFactory(config).createPagePayment(order, returnUrl);
      if (typeof paymentHtml !== 'string' || !paymentHtml.toLowerCase().includes('<form')) {
        throw new Error('支付宝未返回有效支付表单');
      }

      res.status(201).json({
        order: publicOrder(order),
        paymentHtml,
      });
    } catch (error) {
      next(error);
    }
  });

  app.get('/api/coffee/orders/:outTradeNo', async (req, res, next) => {
    try {
      if (!ORDER_NUMBER_PATTERN.test(req.params.outTradeNo)) {
        return res.status(400).json({ error: 'invalid_order_number' });
      }
      const order = await store.getOrder(req.params.outTradeNo);
      if (!order) return res.status(404).json({ error: 'order_not_found' });
      return res.json({ order: publicOrder(order) });
    } catch (error) {
      return next(error);
    }
  });

  app.post('/api/coffee/orders/:outTradeNo/query', async (req, res, next) => {
    try {
      if (!ORDER_NUMBER_PATTERN.test(req.params.outTradeNo)) {
        return res.status(400).json({ error: 'invalid_order_number' });
      }
      const order = await store.getOrder(req.params.outTradeNo);
      if (!order) return res.status(404).json({ error: 'order_not_found' });

      const config = configProvider();
      const result = await clientFactory(config).query(order.outTradeNo);
      if (!isSuccessfulApiResult(result)) {
        return res.status(502).json({
          error: 'alipay_query_failed',
          code: result && result.code ? result.code : '',
          subCode: result && result.sub_code ? result.sub_code : '',
        });
      }
      if (
        result.out_trade_no !== order.outTradeNo ||
        normalizeAmount(result.total_amount) !== normalizeAmount(order.totalAmount)
      ) {
        return res.status(409).json({ error: 'alipay_query_business_mismatch' });
      }

      const updated = await store.updateOrder(order.outTradeNo, current => {
        current.alipayTradeNo = result.trade_no || current.alipayTradeNo || '';
        if (isPaidStatus(result.trade_status)) {
          current.status = 'PAID';
          current.paidAt = current.paidAt || new Date().toISOString();
        } else if (result.trade_status === 'TRADE_CLOSED' && current.status !== 'PAID') {
          current.status = 'CLOSED';
        } else if (result.trade_status === 'WAIT_BUYER_PAY' && current.status === 'CREATED') {
          current.status = 'WAIT_BUYER_PAY';
        }
      });
      return res.json({ order: publicOrder(updated), tradeStatus: result.trade_status || '' });
    } catch (error) {
      return next(error);
    }
  });

  app.post('/api/coffee/orders/:outTradeNo/refunds', async (req, res, next) => {
    try {
      if (req.get('X-1Agents-Confirm') !== 'refund') {
        return res.status(428).json({ error: 'refund_confirmation_required' });
      }
      if (!ORDER_NUMBER_PATTERN.test(req.params.outTradeNo)) {
        return res.status(400).json({ error: 'invalid_order_number' });
      }
      const order = await store.getOrder(req.params.outTradeNo);
      if (!order) return res.status(404).json({ error: 'order_not_found' });
      if (order.status !== 'PAID') {
        return res.status(409).json({ error: 'order_not_paid' });
      }

      const refundAmount = normalizeAmount(req.body && req.body.amount);
      if (!refundAmount || refundAmount === '0.00' || !amountLessThanOrEqual(refundAmount, order.totalAmount)) {
        return res.status(400).json({ error: 'invalid_refund_amount' });
      }
      const suppliedRequestNo = String((req.body && req.body.outRequestNo) || '').trim();
      const outRequestNo = suppliedRequestNo || newId('RF');
      if (!REFUND_NUMBER_PATTERN.test(outRequestNo)) {
        return res.status(400).json({ error: 'invalid_refund_request_number' });
      }

      const reason = String((req.body && req.body.reason) || '用户退款').slice(0, 128);
      const config = configProvider();
      const result = await clientFactory(config).refund(
        order.outTradeNo,
        refundAmount,
        outRequestNo,
        reason
      );
      if (!isSuccessfulApiResult(result)) {
        return res.status(502).json({
          error: 'alipay_refund_failed',
          outRequestNo,
          code: result && result.code ? result.code : '',
          subCode: result && result.sub_code ? result.sub_code : '',
        });
      }

      const updated = await store.updateOrder(order.outTradeNo, current => {
        current.refunds[outRequestNo] = {
          outRequestNo,
          amount: refundAmount,
          status: result.fund_change === 'Y' ? 'REFUND_SUCCESS' : 'PENDING_QUERY',
          createdAt: new Date().toISOString(),
        };
        if (result.fund_change === 'Y' && refundAmount === current.totalAmount) {
          current.status = 'REFUNDED';
        }
      });
      return res.json({
        order: publicOrder(updated),
        outRequestNo,
        fundChange: result.fund_change || '',
      });
    } catch (error) {
      return next(error);
    }
  });

  app.get(
    '/api/coffee/orders/:outTradeNo/refunds/:outRequestNo',
    async (req, res, next) => {
      try {
        if (
          !ORDER_NUMBER_PATTERN.test(req.params.outTradeNo) ||
          !REFUND_NUMBER_PATTERN.test(req.params.outRequestNo)
        ) {
          return res.status(400).json({ error: 'invalid_refund_query' });
        }
        const order = await store.getOrder(req.params.outTradeNo);
        if (!order) return res.status(404).json({ error: 'order_not_found' });

        const config = configProvider();
        const result = await clientFactory(config).queryRefund(
          order.outTradeNo,
          req.params.outRequestNo
        );
        if (!isSuccessfulApiResult(result)) {
          return res.status(502).json({
            error: 'alipay_refund_query_failed',
            code: result && result.code ? result.code : '',
            subCode: result && result.sub_code ? result.sub_code : '',
          });
        }

        const updated = await store.updateOrder(order.outTradeNo, current => {
          const refund = current.refunds[req.params.outRequestNo] || {
            outRequestNo: req.params.outRequestNo,
            amount: normalizeAmount(result.refund_amount) || '',
            createdAt: new Date().toISOString(),
          };
          refund.status = result.refund_status || 'UNKNOWN';
          current.refunds[req.params.outRequestNo] = refund;
          if (
            refund.status === 'REFUND_SUCCESS' &&
            normalizeAmount(refund.amount) === normalizeAmount(current.totalAmount)
          ) {
            current.status = 'REFUNDED';
          }
        });
        return res.json({
          order: publicOrder(updated),
          refundStatus: result.refund_status || '',
        });
      } catch (error) {
        return next(error);
      }
    }
  );

  app.post('/api/coffee/orders/:outTradeNo/close', async (req, res, next) => {
    try {
      if (req.get('X-1Agents-Confirm') !== 'close') {
        return res.status(428).json({ error: 'close_confirmation_required' });
      }
      if (!ORDER_NUMBER_PATTERN.test(req.params.outTradeNo)) {
        return res.status(400).json({ error: 'invalid_order_number' });
      }
      const order = await store.getOrder(req.params.outTradeNo);
      if (!order) return res.status(404).json({ error: 'order_not_found' });
      if (!['CREATED', 'WAIT_BUYER_PAY'].includes(order.status)) {
        return res.status(409).json({ error: 'order_cannot_be_closed' });
      }

      const config = configProvider();
      const result = await clientFactory(config).close(order.outTradeNo);
      if (!isSuccessfulApiResult(result)) {
        return res.status(502).json({
          error: 'alipay_close_failed',
          code: result && result.code ? result.code : '',
          subCode: result && result.sub_code ? result.sub_code : '',
        });
      }
      const updated = await store.updateOrder(order.outTradeNo, current => {
        current.status = 'CLOSED';
        current.alipayTradeNo = result.trade_no || current.alipayTradeNo || '';
      });
      return res.json({ order: publicOrder(updated) });
    } catch (error) {
      return next(error);
    }
  });

  app.get('/coffee/', (req, res) => {
    res.sendFile(path.join(publicDir, 'index.html'));
  });
  app.get('/coffee/return', (req, res) => {
    res.sendFile(path.join(publicDir, 'return.html'));
  });
  app.use('/coffee/', express.static(publicDir, { index: false, fallthrough: false }));

  app.use((error, req, res, next) => {
    if (res.headersSent) return next(error);
    if (error instanceof PaymentConfigError) {
      return res.status(503).json({ error: 'payment_not_configured', message: error.message });
    }
    if (error && error.code === 'INVALID_AMOUNT') {
      return res.status(400).json({ error: 'invalid_amount', message: error.message });
    }
    try {
      logger.error('alipay coffee request failed', {
        method: req.method,
        path: req.path,
        message: error instanceof Error ? error.message : String(error),
      });
    } catch {
      // Keep the HTTP response deterministic even if logging fails.
    }
    return res.status(500).json({ error: 'internal_error', message: '支付服务暂时不可用' });
  });

  return app;
}

module.exports = {
  ORDER_NUMBER_PATTERN,
  REFUND_NUMBER_PATTERN,
  createApp,
  isPaidNotification,
};
