'use strict';

const COFFEE_AMOUNTS = Object.freeze(['6.00', '12.00', '30.00']);

function normalizeAmount(value) {
  const text = String(value ?? '').trim();
  const match = text.match(/^(\d+)(?:\.(\d{1,2}))?$/);
  if (!match) return null;
  const integer = BigInt(match[1]).toString();
  const fraction = (match[2] || '').padEnd(2, '0');
  return `${integer}.${fraction}`;
}

function requireCoffeeAmount(value) {
  const normalized = normalizeAmount(value);
  if (!normalized || !COFFEE_AMOUNTS.includes(normalized)) {
    const error = new Error('请选择有效的咖啡金额');
    error.code = 'INVALID_AMOUNT';
    throw error;
  }
  return normalized;
}

function amountLessThanOrEqual(left, right) {
  const leftText = normalizeAmount(left);
  const rightText = normalizeAmount(right);
  if (!leftText || !rightText) return false;
  const toCents = text => {
    const [integer, fraction] = text.split('.');
    return BigInt(integer) * 100n + BigInt(fraction);
  };
  return toCents(leftText) <= toCents(rightText);
}

module.exports = {
  COFFEE_AMOUNTS,
  normalizeAmount,
  requireCoffeeAmount,
  amountLessThanOrEqual,
};
