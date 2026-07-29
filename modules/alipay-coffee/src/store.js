'use strict';

const fs = require('node:fs/promises');
const path = require('node:path');

function emptyData() {
  return {
    schemaVersion: 1,
    orders: {},
    notifyEvents: {},
  };
}

class JsonOrderStore {
  constructor(filePath) {
    this.filePath = filePath;
    this.queue = Promise.resolve();
  }

  async readData() {
    try {
      const raw = await fs.readFile(this.filePath, 'utf8');
      const parsed = JSON.parse(raw);
      if (
        parsed &&
        parsed.schemaVersion === 1 &&
        parsed.orders &&
        parsed.notifyEvents
      ) {
        return parsed;
      }
      throw new Error('unsupported order store schema');
    } catch (error) {
      if (error && error.code === 'ENOENT') return emptyData();
      throw error;
    }
  }

  async writeData(data) {
    await fs.mkdir(path.dirname(this.filePath), { recursive: true, mode: 0o700 });
    const tempPath = `${this.filePath}.${process.pid}.tmp`;
    await fs.writeFile(tempPath, `${JSON.stringify(data, null, 2)}\n`, {
      encoding: 'utf8',
      mode: 0o600,
    });
    await fs.chmod(tempPath, 0o600);
    await fs.rename(tempPath, this.filePath);
  }

  transact(mutator, { write = true } = {}) {
    const operation = this.queue.then(async () => {
      const data = await this.readData();
      const result = await mutator(data);
      if (write) await this.writeData(data);
      return result;
    });
    this.queue = operation.catch(() => {});
    return operation;
  }

  createOrder(order) {
    return this.transact(data => {
      if (data.orders[order.outTradeNo]) throw new Error('duplicate order number');
      data.orders[order.outTradeNo] = order;
      return structuredClone(order);
    });
  }

  getOrder(outTradeNo) {
    return this.transact(
      data => {
        const order = data.orders[outTradeNo];
        return order ? structuredClone(order) : null;
      },
      { write: false }
    );
  }

  updateOrder(outTradeNo, updater) {
    return this.transact(data => {
      const order = data.orders[outTradeNo];
      if (!order) return null;
      updater(order);
      order.updatedAt = new Date().toISOString();
      return structuredClone(order);
    });
  }

  recordNotificationOnce({ notifyId, params, paid }) {
    return this.transact(data => {
      if (data.notifyEvents[notifyId]) {
        const order = data.orders[params.out_trade_no];
        return { duplicate: true, order: order ? structuredClone(order) : null };
      }

      const order = data.orders[params.out_trade_no];
      if (!order) return { duplicate: false, order: null };

      data.notifyEvents[notifyId] = {
        notifyId,
        outTradeNo: params.out_trade_no,
        tradeNo: params.trade_no || '',
        tradeStatus: params.trade_status || '',
        receivedAt: new Date().toISOString(),
        params,
      };

      if (paid) {
        order.status = 'PAID';
        order.alipayTradeNo = params.trade_no || order.alipayTradeNo || '';
        order.paidAt = order.paidAt || new Date().toISOString();
      } else if (params.trade_status === 'TRADE_CLOSED' && order.status !== 'PAID') {
        order.status = 'CLOSED';
      }
      order.updatedAt = new Date().toISOString();
      return { duplicate: false, order: structuredClone(order) };
    });
  }
}

module.exports = { JsonOrderStore };
