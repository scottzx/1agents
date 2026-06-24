import { defineConfig } from '@tarojs/cli';
import devConfig from './dev';
import prodConfig from './prod';

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
    defineConstants: {},
    copy: {
      patterns: [],
      options: {},
    },
    framework: 'react',
    compiler: 'webpack5',
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
