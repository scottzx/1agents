import { View, Text, Button } from '@tarojs/components';
import Taro from '@tarojs/taro';

import { Screen } from '../../components/Screen';
import { useT } from '../../hooks/useUI';
import './index.scss';

// Home tab — the workspace / session landing. Skeleton for now: the session
// list view (over @1agents/core's workspaceService/agentService) is filled in
// later. The one wired path is "新建对话", which opens the working chat page.
export default function Workspaces() {
  const t = useT();
  const openChat = () => {
    Taro.navigateTo({ url: '/pages/chat/index' });
  };

  return (
    <Screen titleKey="workspaces.title">
      <View className="ws">
        <View className="ws__header">
          <Text className="ws__title">{t('workspaces.title')}</Text>
          <Text className="ws__subtitle">{t('workspaces.subtitle')}</Text>
        </View>

        <View className="ws__body">
          <Text className="ws__hint">{t('workspaces.hint')}</Text>
          <Button className="ws__cta" onClick={openChat}>
            {t('workspaces.newChat')}
          </Button>
        </View>
      </View>
    </Screen>
  );
}
