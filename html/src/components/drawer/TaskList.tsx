import { h } from 'preact';

export function TaskList() {
    return (
        <div class="task-list-container">
            <div class="task-item completed">
                <svg
                    class="check-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 8 12 12 14 14" />
                </svg>
                <span>移除了顶部全局导航栏以呈现 Coze 极简风格</span>
            </div>
            <div class="task-item completed">
                <svg
                    class="check-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 8 12 12 14 14" />
                </svg>
                <span>整合会话头部标题栏，引入运行中动态绿色脉冲灯</span>
            </div>
            <div class="task-item completed">
                <svg
                    class="check-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 8 12 12 14 14" />
                </svg>
                <span>引入 Coze 右上角快捷功能按钮栏 (文件树、任务控制、系统设置)</span>
            </div>
            <div class="task-item completed">
                <svg
                    class="check-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 8 12 12 14 14" />
                </svg>
                <span>实现右侧滑出式抽屉面板 (Quick Drawer System) 及其缓动过渡</span>
            </div>
            <div class="task-item completed">
                <svg
                    class="check-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 8 12 12 14 14" />
                </svg>
                <span>完全兼容并无损保留移动端快捷同步键盘及输入面板</span>
            </div>
        </div>
    );
}
