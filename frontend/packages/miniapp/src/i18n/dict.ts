// Mini-program dictionaries. Keys are namespaced <area>.<purpose>; use {var}
// placeholders for interpolation (see t() in ./index.ts). zh-CN is canonical and
// the fallback for any key missing in en-US. Kept separate from the web's large
// dict (frontend/src/i18n) — the mini-program has its own, smaller string set.

export const zhCN: Record<string, string> = {
  // Common
  'common.skeleton': '页面骨架 · 内容建设中',

  // Tab bar (also surfaced via Taro.setTabBarItem on language change)
  'tab.workspaces': '工作区',
  'tab.providers': '供应商',
  'tab.skills': '技能',
  'tab.more': '更多',

  // Workspaces (home)
  'workspaces.title': '工作区',
  'workspaces.subtitle': '会话与项目',
  'workspaces.hint': '会话列表骨架 · 内容建设中',
  'workspaces.newChat': '新建对话',

  // Providers
  'providers.title': '供应商',
  'providers.desc': 'Claude Code 供应商配置与切换',

  // Skills
  'skills.title': '技能中心',
  'skills.desc': '为协作终端扩展并配置 AI Agent 技能',

  // More
  'more.title': '更多应用',
  'more.subtitle': '分布式协同与高级系统管理',
  'more.discovery': '发现',
  'more.discovery.desc': '分布式协同与应用市场',
  'more.settings': '系统设置',
  'more.settings.desc': '外观 · 后端地址 · 账号',

  // Discovery
  'discovery.title': '发现',
  'discovery.desc': '分布式协同与应用市场',

  // Settings
  'settings.appearance': '外观',
  'settings.language': '语言',
  'settings.theme': '主题',
  'settings.theme.light': '浅色',
  'settings.theme.dark': '深色',
  'settings.connection': '连接',
  'settings.backend': '后端地址',
  'settings.backend.note': '当前为固定地址,可编辑/持久化能力建设中',
  'settings.account': '账号',
  'settings.account.note': '登录与鉴权 · 内容建设中',

  // Chat
  'chat.title': '对话',
  'chat.booting': '正在创建会话…',
  'chat.bootFailed': '启动失败: {error}',
  'chat.noWorkspace': '后端没有可用的 workspace',
  'chat.connection': '连接: {state}',
  'chat.ready': '就绪',
  'chat.startHint': '发条消息开始对话',
  'chat.inputPlaceholder': '输入消息…',
  'chat.inputDisabled': '会话准备中…',
  'chat.send': '发送',
  'chat.sessionName': '小程序对话',
  'chat.perm.request': '{tool} 请求权限',
  'chat.perm.allow': '允许',
  'chat.perm.reject': '拒绝',
};

export const enUS: Record<string, string> = {
  'common.skeleton': 'Page skeleton · under construction',

  'tab.workspaces': 'Workspaces',
  'tab.providers': 'Providers',
  'tab.skills': 'Skills',
  'tab.more': 'More',

  'workspaces.title': 'Workspaces',
  'workspaces.subtitle': 'Sessions & projects',
  'workspaces.hint': 'Session list skeleton · under construction',
  'workspaces.newChat': 'New chat',

  'providers.title': 'Providers',
  'providers.desc': 'Configure and switch Claude Code providers',

  'skills.title': 'Skill center',
  'skills.desc': 'Extend and configure AI agent skills',

  'more.title': 'More',
  'more.subtitle': 'Distributed collaboration & system management',
  'more.discovery': 'Discovery',
  'more.discovery.desc': 'Distributed collaboration & app market',
  'more.settings': 'Settings',
  'more.settings.desc': 'Appearance · backend · account',

  'discovery.title': 'Discovery',
  'discovery.desc': 'Distributed collaboration & app market',

  'settings.appearance': 'Appearance',
  'settings.language': 'Language',
  'settings.theme': 'Theme',
  'settings.theme.light': 'Light',
  'settings.theme.dark': 'Dark',
  'settings.connection': 'Connection',
  'settings.backend': 'Backend',
  'settings.backend.note': 'Currently fixed; editable/persisted support coming',
  'settings.account': 'Account',
  'settings.account.note': 'Login & auth · under construction',

  'chat.title': 'Chat',
  'chat.booting': 'Creating session…',
  'chat.bootFailed': 'Startup failed: {error}',
  'chat.noWorkspace': 'No workspace available on the backend',
  'chat.connection': 'Connection: {state}',
  'chat.ready': 'ready',
  'chat.startHint': 'Send a message to start',
  'chat.inputPlaceholder': 'Type a message…',
  'chat.inputDisabled': 'Session warming up…',
  'chat.send': 'Send',
  'chat.sessionName': 'Mini-program chat',
  'chat.perm.request': '{tool} requests permission',
  'chat.perm.allow': 'Allow',
  'chat.perm.reject': 'Reject',
};
