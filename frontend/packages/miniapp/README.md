# @1agents/miniapp

1Agents mini-program built with **Taro 3 + React + TypeScript**. Weapp
(WeChat mini-program) is the first compile target; React Native is reserved
via `build:rn` for when the app credential lands.

Business logic is reused from [`@1agents/core`](../core) (workspace
dependency); only the view layer is written per platform.

## Scripts

```bash
yarn build:weapp   # build WeChat mini-program -> dist/
yarn dev:weapp     # build + watch
yarn build:rn      # (reserved) React Native target
```

## Status

Skeleton stage (issue #216 §2). The relay transport currently used by the
web frontend relies on `socket.io-client`, which WeChat mini-program does not
support; a native-WebSocket adapter (`Taro.connectSocket`) is wired into
`@1agents/core`'s platform layer in a later step (#216 §3) before Chat can run.
