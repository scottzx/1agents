'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { afterEach, beforeEach, test } = require('node:test');
const { createApp } = require('../src/app');
const { JsonOrderStore } = require('../src/store');

let tempDir;
let server;
let baseUrl;
let store;
let fakeClient;
let calls;

const config = {
  mode: 'sandbox',
  appId: 'sandbox-app',
  privateKey: 'test-private-key',
  alipayPublicKey: 'test-public-key',
  sellerId: '2088000000000000',
  sellerEmail: '',
  gateway: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do',
  notifyUrl: '',
  publicOrigin: '',
};

beforeEach(async () => {
  tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'alipay-coffee-test-'));
  store = new JsonOrderStore(path.join(tempDir, 'orders.json'));
  calls = [];
  fakeClient = {
    createPagePayment(order, returnUrl) {
      calls.push({ method: 'create', order, returnUrl });
      return '<form method="post" action="https://openapi-sandbox.dl.alipaydev.com/gateway.do"></form>';
    },
    async query(outTradeNo) {
      const order = await store.getOrder(outTradeNo);
      calls.push({ method: 'query', outTradeNo });
      return {
        code: '10000',
        out_trade_no: outTradeNo,
        trade_no: '2026072900000001',
        total_amount: order.totalAmount,
        trade_status: 'TRADE_SUCCESS',
      };
    },
    async refund(outTradeNo, amount, outRequestNo) {
      calls.push({ method: 'refund', outTradeNo, amount, outRequestNo });
      return { code: '10000', fund_change: 'Y', out_trade_no: outTradeNo };
    },
    async queryRefund(outTradeNo, outRequestNo) {
      calls.push({ method: 'refund-query', outTradeNo, outRequestNo });
      return {
        code: '10000',
        out_trade_no: outTradeNo,
        out_request_no: outRequestNo,
        refund_amount: '6.00',
        refund_status: 'REFUND_SUCCESS',
      };
    },
    async close(outTradeNo) {
      calls.push({ method: 'close', outTradeNo });
      return { code: '10000', out_trade_no: outTradeNo };
    },
    verifyNotification(params) {
      calls.push({ method: 'notify-verify', params });
      return params.sign === 'valid-signature';
    },
  };

  const app = createApp({
    configProvider: () => config,
    clientFactory: () => fakeClient,
    store,
    logger: { error() {}, warn() {} },
  });
  server = app.listen(0, '127.0.0.1');
  await new Promise(resolve => server.once('listening', resolve));
  const address = server.address();
  baseUrl = `http://127.0.0.1:${address.port}`;
});

afterEach(async () => {
  if (server) await new Promise(resolve => server.close(resolve));
  if (tempDir) await fs.rm(tempDir, { recursive: true, force: true });
});

async function createOrder(amount = '6.00') {
  const response = await fetch(`${baseUrl}/api/coffee/orders`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ amount }),
  });
  const body = await response.json();
  return { response, body };
}

test('creates a persistent order and returns an Alipay HTML form', async () => {
  const { response, body } = await createOrder();
  assert.equal(response.status, 201);
  assert.match(body.order.outTradeNo, /^COFFEE[0-9A-F]+$/);
  assert.equal(body.order.totalAmount, '6.00');
  assert.equal(body.order.status, 'CREATED');
  assert.match(body.paymentHtml, /<form/);

  const persisted = await store.getOrder(body.order.outTradeNo);
  assert.equal(persisted.totalAmount, '6.00');
  assert.equal(calls[0].method, 'create');
  assert.match(calls[0].returnUrl, /\/coffee\/return\?out_trade_no=COFFEE/);
});

test('rejects amounts outside the server-side coffee presets', async () => {
  const { response, body } = await createOrder('0.01');
  assert.equal(response.status, 400);
  assert.equal(body.error, 'invalid_amount');
  assert.equal(calls.length, 0);
});

test('queries Alipay and marks the matching order paid', async () => {
  const created = await createOrder('12.00');
  const outTradeNo = created.body.order.outTradeNo;
  const response = await fetch(`${baseUrl}/api/coffee/orders/${outTradeNo}/query`, {
    method: 'POST',
  });
  const body = await response.json();

  assert.equal(response.status, 200);
  assert.equal(body.order.status, 'PAID');
  assert.equal(body.order.alipayTradeNo, '2026072900000001');
  assert.equal(body.tradeStatus, 'TRADE_SUCCESS');
});

test('verifies, validates and idempotently persists payment notifications', async () => {
  const created = await createOrder('30.00');
  const outTradeNo = created.body.order.outTradeNo;
  const params = new URLSearchParams({
    notify_type: 'trade_status_sync',
    notify_id: 'notify-1',
    sign_type: 'RSA2',
    sign: 'valid-signature',
    trade_no: '2026072900000002',
    app_id: config.appId,
    out_trade_no: outTradeNo,
    trade_status: 'TRADE_SUCCESS',
    total_amount: '30.00',
    seller_id: config.sellerId,
  });

  for (let attempt = 0; attempt < 2; attempt += 1) {
    const response = await fetch(`${baseUrl}/api/coffee/notify`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: params,
    });
    assert.equal(response.status, 200);
    assert.equal(await response.text(), 'success');
  }

  const order = await store.getOrder(outTradeNo);
  assert.equal(order.status, 'PAID');
  assert.equal(order.alipayTradeNo, '2026072900000002');
  const persisted = JSON.parse(await fs.readFile(path.join(tempDir, 'orders.json'), 'utf8'));
  assert.equal(Object.keys(persisted.notifyEvents).length, 1);
  assert.equal('sign' in persisted.notifyEvents['notify-1'].params, false);
});

test('requires explicit confirmation for refund and close operations', async () => {
  const created = await createOrder('6.00');
  const outTradeNo = created.body.order.outTradeNo;

  const closeWithoutConfirmation = await fetch(
    `${baseUrl}/api/coffee/orders/${outTradeNo}/close`,
    { method: 'POST' }
  );
  assert.equal(closeWithoutConfirmation.status, 428);

  const queryResponse = await fetch(`${baseUrl}/api/coffee/orders/${outTradeNo}/query`, {
    method: 'POST',
  });
  assert.equal(queryResponse.status, 200);

  const refundWithoutConfirmation = await fetch(
    `${baseUrl}/api/coffee/orders/${outTradeNo}/refunds`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ amount: '6.00' }),
    }
  );
  assert.equal(refundWithoutConfirmation.status, 428);

  const refundResponse = await fetch(`${baseUrl}/api/coffee/orders/${outTradeNo}/refunds`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-1Agents-Confirm': 'refund',
    },
    body: JSON.stringify({ amount: '6.00' }),
  });
  const refundBody = await refundResponse.json();
  assert.equal(refundResponse.status, 200);
  assert.equal(refundBody.order.status, 'REFUNDED');
  assert.match(refundBody.outRequestNo, /^RF[0-9A-F]+$/);
});

test('serves a neutral return page without trusting callback parameters', async () => {
  const response = await fetch(`${baseUrl}/coffee/return`);
  const body = await response.text();
  assert.equal(response.status, 200);
  assert.match(body, /正在确认支付结果/);
  assert.match(body, /不会直接使用浏览器回跳参数判断付款成功/);
});
