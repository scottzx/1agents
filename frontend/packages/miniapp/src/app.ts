import {PropsWithChildren} from 'react';
import {useLaunch} from '@tarojs/taro';

import './app.scss';

function App({children}: PropsWithChildren) {
  useLaunch(() => {
    console.log('App launched.');
  });

  // children is the page rendered by Taro's router.
  return children;
}

export default App;
