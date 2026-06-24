export default defineAppConfig({
  // Launch page is the first tabBar entry (工作区). Sub-pages (chat / tasks /
  // discovery / settings) are reached via Taro.navigateTo, not the tabBar.
  pages: [
    'pages/workspaces/index',
    'pages/providers/index',
    'pages/skills/index',
    'pages/more/index',
    'pages/chat/index',
    'pages/tasks/index',
    'pages/discovery/index',
    'pages/settings/index',
  ],
  window: {
    backgroundTextStyle: 'light',
    navigationBarBackgroundColor: '#070b14',
    navigationBarTitleText: '1Agents',
    navigationBarTextStyle: 'white',
  },
  tabBar: {
    color: '#5f7390',
    selectedColor: '#00aaff',
    backgroundColor: '#070b14',
    borderStyle: 'black',
    list: [
      { pagePath: 'pages/workspaces/index', text: '工作区' },
      { pagePath: 'pages/providers/index', text: '供应商' },
      { pagePath: 'pages/skills/index', text: '技能' },
      { pagePath: 'pages/more/index', text: '更多' },
    ],
  },
});
