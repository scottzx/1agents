// 供应商 tab — cc-connect's 服务商/模型配置 surface: the same cc-connect web-view
// as 渠道, just with path='/providers'. Two front-ends, one embedded module,
// distinguished by the route parameter.
import { CcConnectWebView } from '../../components/CcConnectWebView';

export default function Providers() {
  return <CcConnectWebView path="/providers" titleKey="tab.providers" />;
}
