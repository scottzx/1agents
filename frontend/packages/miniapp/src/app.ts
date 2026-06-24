import {PropsWithChildren} from 'react';
import {useLaunch} from '@tarojs/taro';
import {setPlatformBridge} from '@1agents/core/platform/bridge';
import {setDirectBackend} from '@1agents/core/services/apiClient';

import {TaroPlatformBridge} from './platform/taroBridge';
import {BACKEND_BASE} from './config';
import './app.scss';

// Install the Taro-backed platform bridge once, before any core service runs,
// so core's transport uses Taro.connectSocket / Taro APIs on this host. Then
// point the API client at the configured backend (direct mode, absolute base).
setPlatformBridge(new TaroPlatformBridge());
setDirectBackend(BACKEND_BASE);

function App({children}: PropsWithChildren) {
  useLaunch(() => {
    console.log('App launched.');
  });

  // children is the page rendered by Taro's router.
  return children;
}

export default App;
