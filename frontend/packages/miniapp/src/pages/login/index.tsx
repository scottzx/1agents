// 登录页(手机号 + 验证码)。脚手架阶段:后端 dev 固定验证码 123456,手机号
// 只过闸不落库(先不绑)。微信一键登录留按钮位,接真 getPhoneNumber 时再补
// (需企业小程序 AppID/AppSecret,后端 wx.login code 换 session_key 解手机号)。
import { useState } from 'react';
import { View, Text, Input, Button } from '@tarojs/components';
import type { BaseEventOrig, InputProps } from '@tarojs/components';
import Taro from '@tarojs/taro';

import { Screen } from '../../components/Screen';
import { getBackendBase } from '../../config';
import { sendCode, verifyPhone, InvalidCodeError } from '@1agents/core/services/authService';
import './index.scss';

const PHONE_RE = /^\+?\d{6,15}$/;

type InputEvent = BaseEventOrig<InputProps.inputEventDetail>;

export default function Login() {
  const [phone, setPhone] = useState('');
  const [code, setCode] = useState('');
  const [sending, setSending] = useState(false);
  const [countdown, setCountdown] = useState(0);
  const [loggingIn, setLoggingIn] = useState(false);
  const [error, setError] = useState('');

  const startCountdown = () => {
    setCountdown(60);
    const timer = setInterval(() => {
      setCountdown((n) => {
        if (n <= 1) {
          clearInterval(timer);
          return 0;
        }
        return n - 1;
      });
    }, 1000);
  };

  const handleSendCode = async () => {
    setError('');
    if (!PHONE_RE.test(phone.trim())) {
      setError('请输入正确的手机号');
      return;
    }
    setSending(true);
    try {
      await sendCode(getBackendBase(), phone.trim());
      startCountdown();
      Taro.showToast({ title: '验证码已发送(开发期固定 123456)', icon: 'none' });
    } catch (e) {
      setError((e as Error).message || '发送失败');
    } finally {
      setSending(false);
    }
  };

  const handleLogin = async () => {
    setError('');
    if (!PHONE_RE.test(phone.trim())) {
      setError('请输入正确的手机号');
      return;
    }
    if (!/^\d{6}$/.test(code.trim())) {
      setError('请输入 6 位验证码');
      return;
    }
    setLoggingIn(true);
    try {
      await verifyPhone(getBackendBase(), phone.trim(), code.trim());
      Taro.showToast({ title: '登录成功', icon: 'success' });
      Taro.reLaunch({ url: '/pages/workspaces/index' });
    } catch (e) {
      setError(e instanceof InvalidCodeError ? '验证码错误' : (e as Error).message || '登录失败');
    } finally {
      setLoggingIn(false);
    }
  };

  const handleWxLogin = () => {
    Taro.showToast({ title: '微信一键登录即将接入', icon: 'none' });
  };

  return (
    <Screen className="login">
      <View className="login__head">
        <Text className="login__title">登录 1Agents</Text>
        <Text className="login__sub">手机号 + 验证码登录</Text>
      </View>

      <View className="login__form">
        <View className="login__field">
          <Input
            className="login__input"
            type="number"
            maxlength={15}
            placeholder="请输入手机号"
            value={phone}
            onInput={(e: InputEvent) => setPhone(e.detail.value)}
          />
        </View>

        <View className="login__field login__field--code">
          <Input
            className="login__input"
            type="number"
            maxlength={6}
            placeholder="6 位验证码"
            value={code}
            onInput={(e: InputEvent) => setCode(e.detail.value)}
          />
          <Button className="login__code-btn" size="mini" disabled={sending || countdown > 0} onClick={handleSendCode}>
            {countdown > 0 ? `${countdown}s` : '获取验证码'}
          </Button>
        </View>

        {error ? <Text className="login__error">{error}</Text> : null}

        <Button className="login__submit" type="primary" loading={loggingIn} onClick={handleLogin}>
          登录
        </Button>

        <View className="login__divider">
          <Text className="login__divider-text">或</Text>
        </View>

        <Button className="login__wx" onClick={handleWxLogin}>
          微信一键登录
        </Button>
      </View>
    </Screen>
  );
}
