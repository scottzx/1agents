'use strict';

const amountsElement = document.querySelector('#amounts');
const payButton = document.querySelector('#pay-button');
const payAmount = document.querySelector('#pay-amount');
const environmentBadge = document.querySelector('#environment-badge');
const statusPanel = document.querySelector('#status');
const statusText = document.querySelector('#status-text');
const orderActions = document.querySelector('#order-actions');
const queryButton = document.querySelector('#query-button');
const orderNumber = document.querySelector('#order-number');
const formContainer = document.querySelector('#payment-form-container');

let selectedAmount = '6.00';
let currentOrder = null;
let pollTimer = null;

function setTheme(theme) {
  document.documentElement.dataset.theme = theme === 'dark' ? 'dark' : 'light';
}

setTheme(window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
window.addEventListener('message', event => {
  if (event.origin !== window.location.origin) return;
  if (event.data && event.data.type === '1agents-theme') setTheme(event.data.theme);
});

function setStatus(kind, text) {
  statusPanel.dataset.kind = kind;
  statusText.textContent = text;
}

function renderAmounts(amounts) {
  amountsElement.replaceChildren();
  for (const amount of amounts) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `amount-button${amount === selectedAmount ? ' selected' : ''}`;
    button.setAttribute('role', 'radio');
    button.setAttribute('aria-checked', String(amount === selectedAmount));
    button.dataset.amount = amount;
    button.innerHTML = `<span>¥</span><strong>${amount}</strong>`;
    button.addEventListener('click', () => {
      selectedAmount = amount;
      payAmount.textContent = `¥${amount}`;
      for (const candidate of amountsElement.querySelectorAll('.amount-button')) {
        const selected = candidate.dataset.amount === amount;
        candidate.classList.toggle('selected', selected);
        candidate.setAttribute('aria-checked', String(selected));
      }
    });
    amountsElement.appendChild(button);
  }
}

function orderStatusText(order) {
  switch (order.status) {
    case 'PAID':
      return '支付成功，谢谢你的咖啡 ☕';
    case 'REFUNDED':
      return '这笔咖啡订单已退款';
    case 'CLOSED':
      return '订单已关闭';
    case 'WAIT_BUYER_PAY':
      return '等待在支付宝收银台完成付款';
    default:
      return '订单已创建，等待支付';
  }
}

function updateOrder(order) {
  currentOrder = order;
  window.sessionStorage.setItem('alipayCoffeeOrder', order.outTradeNo);
  orderActions.hidden = false;
  orderNumber.textContent = `订单 ${order.outTradeNo}`;
  const completed = ['PAID', 'REFUNDED', 'CLOSED'].includes(order.status);
  payButton.disabled = !completed;
  setStatus(order.status === 'PAID' ? 'success' : completed ? 'muted' : 'pending', orderStatusText(order));
  if (completed && pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function queryOrder() {
  if (!currentOrder) return;
  queryButton.disabled = true;
  try {
    const response = await fetch(
      `/api/coffee/orders/${encodeURIComponent(currentOrder.outTradeNo)}/query`,
      { method: 'POST', headers: { Accept: 'application/json' } }
    );
    const data = await response.json();
    if (!response.ok) throw new Error(data.message || data.error || `HTTP ${response.status}`);
    updateOrder(data.order);
  } catch (error) {
    setStatus('error', `暂时无法查询支付结果：${error.message}`);
  } finally {
    queryButton.disabled = false;
  }
}

function startPolling() {
  if (pollTimer) window.clearInterval(pollTimer);
  pollTimer = window.setInterval(() => void queryOrder(), 5000);
  window.setTimeout(() => {
    if (pollTimer) {
      window.clearInterval(pollTimer);
      pollTimer = null;
    }
  }, 5 * 60 * 1000);
}

async function createPayment() {
  const checkoutName = `alipayCoffeeCheckout${Date.now()}`;
  const checkout = window.open('', checkoutName);
  if (!checkout) {
    setStatus('error', '浏览器阻止了支付窗口，请允许弹出窗口后重试。');
    return;
  }
  checkout.document.title = '正在打开支付宝收银台';
  checkout.document.body.textContent = '正在打开支付宝收银台…';

  payButton.disabled = true;
  setStatus('pending', '正在安全创建订单…');
  try {
    const response = await fetch('/api/coffee/orders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ amount: selectedAmount }),
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.message || data.error || `HTTP ${response.status}`);

    updateOrder(data.order);
    formContainer.innerHTML = data.paymentHtml;
    const form = formContainer.querySelector('form');
    if (!form) throw new Error('支付宝支付表单无效');
    form.target = checkoutName;
    form.submit();
    setStatus('pending', '支付宝收银台已打开，完成付款后可在这里查询结果。');
    startPolling();
  } catch (error) {
    checkout.close();
    setStatus('error', `无法发起支付：${error.message}`);
  } finally {
    formContainer.replaceChildren();
    payButton.disabled =
      currentOrder !== null && !['PAID', 'REFUNDED', 'CLOSED'].includes(currentOrder.status);
  }
}

async function restorePendingOrder() {
  const outTradeNo = window.sessionStorage.getItem('alipayCoffeeOrder') || '';
  if (!/^COFFEE[0-9A-F]{20,40}$/.test(outTradeNo)) return false;
  try {
    const response = await fetch(`/api/coffee/orders/${encodeURIComponent(outTradeNo)}`, {
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) return false;
    const data = await response.json();
    updateOrder(data.order);
    if (!['PAID', 'REFUNDED', 'CLOSED'].includes(data.order.status)) {
      await queryOrder();
      if (currentOrder && !['PAID', 'REFUNDED', 'CLOSED'].includes(currentOrder.status)) {
        startPolling();
      }
    }
    return true;
  } catch {
    return false;
  }
}

async function initialize() {
  try {
    const response = await fetch('/api/coffee/config', { headers: { Accept: 'application/json' } });
    const config = await response.json();
    renderAmounts(config.amounts || ['6.00', '12.00', '30.00']);
    environmentBadge.textContent = config.mode === 'production' ? '正式环境' : '沙箱环境';
    environmentBadge.dataset.mode = config.mode;
    if (!config.ready) {
      payButton.disabled = true;
      setStatus('error', config.message || '支付配置尚未完成');
      return;
    }
    setStatus(
      'ready',
      config.notifyEnabled ? '支付服务已就绪' : '本地验收模式：公网异步通知尚未配置'
    );
    const restored = await restorePendingOrder();
    if (!restored) payButton.disabled = false;
  } catch (error) {
    renderAmounts(['6.00', '12.00', '30.00']);
    setStatus('error', `支付子服务不可用：${error.message}`);
  }
}

payButton.addEventListener('click', () => void createPayment());
queryButton.addEventListener('click', () => void queryOrder());
void initialize();
