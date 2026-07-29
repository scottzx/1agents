'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { createAlipayClient } = require('../src/alipay-client');

test('uses the installed SDK pageExec and checkNotifySignV2 contracts', async () => {
  const calls = [];
  class FakeSdk {
    constructor(config) {
      calls.push({ method: 'constructor', config });
    }

    pageExec(method, httpMethod, options) {
      calls.push({ method: 'pageExec', api: method, httpMethod, options });
      return '<form></form>';
    }

    exec(method, options) {
      calls.push({ method: 'exec', api: method, options });
      return Promise.resolve({ code: '10000' });
    }

    checkNotifySignV2(params) {
      calls.push({ method: 'checkNotifySignV2', params });
      return true;
    }
  }

  const config = {
    appId: 'app-id',
    privateKey: 'private-key',
    alipayPublicKey: 'public-key',
    gateway: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do',
    notifyUrl: 'https://example.test/api/coffee/notify',
  };
  const client = createAlipayClient(config, FakeSdk);
  const order = {
    outTradeNo: 'COFFEE123',
    totalAmount: '6.00',
    subject: '请作者喝杯咖啡',
  };

  assert.equal(client.createPagePayment(order, 'http://localhost/coffee/return'), '<form></form>');
  await client.query(order.outTradeNo);
  assert.equal(client.verifyNotification({ sign: 'value' }), true);

  assert.deepEqual(calls[0].config, {
    appId: 'app-id',
    privateKey: 'private-key',
    alipayPublicKey: 'public-key',
    gateway: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do',
    signType: 'RSA2',
    keyType: 'PKCS1',
    camelcase: false,
  });
  assert.equal(calls[1].method, 'pageExec');
  assert.equal(calls[1].api, 'alipay.trade.page.pay');
  assert.equal(calls[1].httpMethod, 'POST');
  assert.equal(calls[1].options.notifyUrl, config.notifyUrl);
  assert.equal(calls[2].api, 'alipay.trade.query');
  assert.equal(calls[3].method, 'checkNotifySignV2');
});
