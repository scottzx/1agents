'use strict';

const { createApp } = require('./app');

const host = process.env.ALIPAY_COFFEE_HOST || '127.0.0.1';
const port = Number.parseInt(process.env.ALIPAY_COFFEE_PORT || '38087', 10);
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('ALIPAY_COFFEE_PORT 必须是有效端口');
}

const server = createApp().listen(port, host, () => {
  console.log(`[alipay-coffee] listening on http://${host}:${port}`);
});

function shutdown() {
  server.close(error => {
    if (error) {
      console.error('[alipay-coffee] shutdown failed:', error.message);
      process.exitCode = 1;
    }
  });
}

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
