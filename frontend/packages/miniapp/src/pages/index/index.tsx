import {useState} from 'react';
import {View, Text} from '@tarojs/components';
import {getPlatformBridge} from '@1agents/core/platform/bridge';
// Type-only import from the shared core package proves the @1agents/core
// workspace dependency resolves under Taro's TS build.
import type {ConnectionState} from '@1agents/core/protocol/types';

import './index.scss';

export default function Index() {
  const [status] = useState<ConnectionState>('idle');
  // The Taro bridge is injected at app launch (app.ts). Confirm core resolves
  // it and exposes the Taro.connectSocket-backed transport on this host.
  const transportReady = typeof getPlatformBridge().connectSocket === 'function';

  return (
    <View className="index">
      <Text className="index__title">1Agents</Text>
      <Text className="index__subtitle">Taro mini-program skeleton</Text>
      <Text className="index__status">core link OK · status: {status}</Text>
      <Text className="index__status">transport: {transportReady ? 'Taro.connectSocket ✓' : 'unavailable'}</Text>
    </View>
  );
}
