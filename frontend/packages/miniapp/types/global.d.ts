/// <reference types="@tarojs/taro" />

// Build-time base path constant referenced by @1agents/core's apiClient
// (web injects it via webpack DefinePlugin; here Taro defineConstants sets '').
declare const __BASE_PATH__: string;

declare module '*.png';
declare module '*.gif';
declare module '*.jpg';
declare module '*.jpeg';
declare module '*.svg';
declare module '*.css';
declare module '*.less';
declare module '*.scss';
declare module '*.sass';
declare module '*.styl';

// @ts-ignore
declare const process: {
  env: {
    NODE_ENV: 'development' | 'production';
    TARO_ENV:
      | 'weapp'
      | 'swan'
      | 'alipay'
      | 'h5'
      | 'rn'
      | 'tt'
      | 'quickapp'
      | 'qq'
      | 'jd';
    [key: string]: any;
  };
};
