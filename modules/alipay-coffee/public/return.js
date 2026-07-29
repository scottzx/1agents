'use strict';

const title = document.querySelector('#return-title');
const message = document.querySelector('#return-message');
const queryButton = document.querySelector('#return-query');
const outTradeNo = new URLSearchParams(window.location.search).get('out_trade_no') || '';
const validOrderNumber = /^COFFEE[0-9A-F]{20,40}$/.test(outTradeNo);

async function query() {
  if (!validOrderNumber) {
    title.textContent = '等待订单信息';
    message.textContent = '当前回跳地址没有可验证的订单，请返回设置页查询支付结果。';
    queryButton.hidden = true;
    return;
  }

  queryButton.disabled = true;
  title.textContent = '正在确认支付结果';
  message.textContent = '请稍候，我们正在通过服务端向支付宝查询这笔订单。';
  try {
    const response = await fetch(`/api/coffee/orders/${encodeURIComponent(outTradeNo)}/query`, {
      method: 'POST',
      headers: { Accept: 'application/json' },
    });
    const data = await response.json();
    if (!response.ok) throw new Error(data.message || data.error || `HTTP ${response.status}`);
    if (data.order.status === 'PAID') {
      title.textContent = '支付成功，谢谢你的咖啡 ☕';
      message.textContent = '服务端已向支付宝确认付款成功，可以安全关闭此页面。';
      queryButton.hidden = true;
    } else if (data.order.status === 'CLOSED') {
      title.textContent = '订单已关闭';
      message.textContent = '支付宝确认这笔订单已经关闭。';
    } else {
      title.textContent = '暂未确认付款';
      message.textContent = '支付宝尚未返回付款成功状态。如果刚完成支付，请稍后重新查询。';
    }
  } catch (error) {
    title.textContent = '暂时无法确认支付结果';
    message.textContent = `请返回设置页稍后再试：${error.message}`;
  } finally {
    queryButton.disabled = false;
  }
}

queryButton.addEventListener('click', () => void query());
void query();
