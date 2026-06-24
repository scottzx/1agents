import {useState} from 'react';
import {View, Text} from '@tarojs/components';
// Type-only import from the shared core package proves the @1agents/core
// workspace dependency resolves under Taro's TS build. Runtime transport
// (native WebSocket adapter replacing socket.io for weapp) is wired in a
// later step — see issue #216 §3.
import type {ConnectionState} from '@1agents/core/protocol/types';

import './index.scss';

export default function Index() {
  const [status] = useState<ConnectionState>('idle');

  return (
    <View className="index">
      <Text className="index__title">1Agents</Text>
      <Text className="index__subtitle">Taro mini-program skeleton</Text>
      <Text className="index__status">core link OK · status: {status}</Text>
    </View>
  );
}
