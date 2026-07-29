import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import type { Lang } from '../../i18n';

interface CoffeePanelProps {
    language: Lang;
    theme: 'light' | 'dark';
}

export function CoffeePanel({ language, theme }: CoffeePanelProps) {
    const frameRef = useRef<HTMLIFrameElement>(null);

    useEffect(() => {
        frameRef.current?.contentWindow?.postMessage(
            { type: '1agents-theme', theme, language },
            window.location.origin
        );
    }, [language, theme]);

    return (
        <div class="sys-settings-section sys-settings-section--coffee">
            <div class="sys-settings-section-title">
                {language === 'zh-CN' ? '请作者喝杯咖啡' : 'Buy the author a coffee'}
            </div>
            <div class="sys-settings-section-desc">
                {language === 'zh-CN'
                    ? '通过支付宝支持 1Agents 的持续开发。支付页面安全地嵌入当前设置界面。'
                    : 'Support the continued development of 1Agents with Alipay.'}
            </div>
            <iframe
                ref={frameRef}
                class="coffee-settings-frame"
                src="/coffee/"
                title={language === 'zh-CN' ? '请作者喝杯咖啡支付页' : 'Coffee support payment'}
                sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox"
                referrerPolicy="no-referrer"
                onLoad={() =>
                    frameRef.current?.contentWindow?.postMessage(
                        { type: '1agents-theme', theme, language },
                        window.location.origin
                    )
                }
            />
        </div>
    );
}
