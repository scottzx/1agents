import * as path from 'path';
import { defineConfig } from '@tarojs/cli';
import devConfig from './dev';
import prodConfig from './prod';

// @1agents/core ships TypeScript source and is consumed as a workspace symlink,
// so its files resolve under packages/core (outside src/). Taro only transpiles
// src/ by default, so force its compiler to also process the core package —
// otherwise the runtime (value) imports from core fail to parse as plain JS.
// Use the real resolved path (Taro sets resolve.symlinks=true).
const corePath = path.resolve(__dirname, '..', '..', 'core');

// Taro 3 (weapp-first) base config. RN target is reserved via `build:rn`;
// view layer is written per-platform while business logic comes from
// @1agents/core (workspace dependency).
export default defineConfig(async merge => {
  const baseConfig = {
    projectName: 'miniapp',
    date: '2026-6-24',
    designWidth: 750,
    deviceRatio: {
      640: 2.34 / 2,
      750: 1,
      375: 2,
      828: 1.81 / 2,
    },
    sourceRoot: 'src',
    outputRoot: 'dist',
    plugins: [],
    // __BASE_PATH__ is a web-only build constant referenced by @1agents/core's
    // apiClient (relay default origin). The mini-program has no subpath mount, so
    // define it as '' to keep the shared module compiling/running here.
    defineConstants: {
      __BASE_PATH__: JSON.stringify(''),
    },
    copy: {
      patterns: [],
      options: {},
    },
    framework: 'react',
    // Object form so prebundle can be disabled: the esbuild prebundle scans
    // node_modules deps (incl. the symlinked @1agents/core) and bypasses the
    // babel rule, leaving core's TS unparsed. Disabling it routes core through
    // the normal pipeline together with compile.include below.
    compiler: {
      type: 'webpack5',
      prebundle: { enable: false },
    },
    cache: {
      enable: false,
    },
    // Disable CSS minification (csso). Both csso and esbuild minifiers crash
    // under this Taro 3.6.40 / Node 22 toolchain (csso: "reading 'Tag'";
    // esbuild: invalid "preset" option). JS minification (terser) still runs.
    // Revisit when bumping Taro / css-minimizer-webpack-plugin.
    csso: {
      enable: false,
    },
    mini: {
      postcss: {
        pxtransform: {
          enable: true,
          config: {},
        },
        cssModules: {
          enable: false,
        },
      },
      // Force the babel 'script' rule to also transpile @1agents/core's TS
      // source. Match both the resolved real path (packages/core) and the
      // workspace-symlink path (node_modules/@1agents/core) since webpack may
      // present either to the rule's include matcher.
      webpackChain(chain: any) {
        chain.module
          .rule('script')
          .include.add(corePath)
          .add(/node_modules[\\/]@1agents[\\/]core[\\/]/)
          .end();
      },
    },
    h5: {
      publicPath: '/',
      staticDirectory: 'static',
      postcss: {
        autoprefixer: {
          enable: true,
          config: {},
        },
        cssModules: {
          enable: false,
        },
      },
    },
  };

  if (process.env.NODE_ENV === 'development') {
    return merge({}, baseConfig, devConfig);
  }
  return merge({}, baseConfig, prodConfig);
});
