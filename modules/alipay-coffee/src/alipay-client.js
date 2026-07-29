'use strict';

const { AlipaySdk } = require('alipay-sdk');

function createAlipayClient(config, Sdk = AlipaySdk) {
  const sdk = new Sdk({
    appId: config.appId,
    privateKey: config.privateKey,
    alipayPublicKey: config.alipayPublicKey,
    gateway: config.gateway,
    signType: 'RSA2',
    keyType: 'PKCS1',
    camelcase: false,
  });

  return {
    createPagePayment(order, returnUrl) {
      const options = {
        returnUrl,
        bizContent: {
          out_trade_no: order.outTradeNo,
          total_amount: order.totalAmount,
          subject: order.subject,
          product_code: 'FAST_INSTANT_TRADE_PAY',
        },
      };
      if (config.notifyUrl) options.notifyUrl = config.notifyUrl;
      return sdk.pageExec('alipay.trade.page.pay', 'POST', options);
    },

    query(outTradeNo) {
      return sdk.exec('alipay.trade.query', {
        bizContent: { out_trade_no: outTradeNo },
      });
    },

    refund(outTradeNo, amount, outRequestNo, reason) {
      return sdk.exec('alipay.trade.refund', {
        bizContent: {
          out_trade_no: outTradeNo,
          refund_amount: amount,
          refund_reason: reason,
          out_request_no: outRequestNo,
        },
      });
    },

    queryRefund(outTradeNo, outRequestNo) {
      return sdk.exec('alipay.trade.fastpay.refund.query', {
        bizContent: {
          out_trade_no: outTradeNo,
          out_request_no: outRequestNo,
        },
      });
    },

    close(outTradeNo) {
      return sdk.exec('alipay.trade.close', {
        bizContent: { out_trade_no: outTradeNo },
      });
    },

    verifyNotification(params) {
      return sdk.checkNotifySignV2(params);
    },
  };
}

module.exports = { createAlipayClient };
