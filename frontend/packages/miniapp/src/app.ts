import {PropsWithChildren} from 'react';
import {useLaunch} from '@tarojs/taro';
import {setPlatformBridge} from '@1agents/core/platform/bridge';

import {TaroPlatformBridge} from './platform/taroBridge';
import './app.scss';

// Install the Taro-backed platform bridge once, before any core service runs,
// so core's transport uses Taro.connectSocket / Taro APIs on this host.
setPlatformBridge(new TaroPlatformBridge());

function App({children}: PropsWithChildren) {
  useLaunch(() => {
    console.log('App launched.');
  });

  // children is the page rendered by Taro's router.
  return children;
}

export default App;
