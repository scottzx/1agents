// Shared cc-connect <web-view>. cc-connect has two surfaces that are the SAME
// embedded module differing only by route: 渠道 (channels, no path) and 服务商配置
// (model/provider config, path='/providers'). Both bootstrap the first workspace,
// ask the backend to mint the tokened login URL for that path
// (workspaceService.getCcConnectUrl — the ManagementToken is baked in there),
// absolutize it onto BACKEND_BASE and carry the access token for the gate. The
// two pages just pass a different `path`.
import { WebView } from '@tarojs/components';
import { useEffect, useState } from 'react';

import { workspaceService } from '@1agents/core/services/workspaceService';

import { Screen } from './Screen';
import { Loading } from './ui/Loading';
import { EmptyState } from './ui/EmptyState';
import { useT, useUI } from '../hooks/useUI';
import { absoluteBackendUrl } from '../embed';

export interface CcConnectWebViewProps {
  /** Route inside cc-connect: '' = 渠道 (channels), '/providers' = 服务商/模型配置. */
  path?: string;
  /** i18n key for the loading/error screen's nav title. */
  titleKey: string;
}

export function CcConnectWebView({ path, titleKey }: CcConnectWebViewProps) {
  const t = useT();
  const { theme, lang } = useUI();
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const wss = await workspaceService.list();
        const ws = wss[0];
        if (!ws) {
          if (!cancelled) setError(t('ccConnect.noWorkspace'));
          return;
        }
        const rel = await workspaceService.getCcConnectUrl(ws.id, theme, lang, path);
        if (!cancelled) setUrl(absoluteBackendUrl(rel));
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path]);

  if (error) {
    return (
      <Screen titleKey={titleKey}>
        <EmptyState icon="⚠️" title={t('ccConnect.error')} desc={error} />
      </Screen>
    );
  }
  if (!url) {
    return (
      <Screen titleKey={titleKey}>
        <Loading text={t('ccConnect.loading')} />
      </Screen>
    );
  }
  return <WebView src={url} />;
}
