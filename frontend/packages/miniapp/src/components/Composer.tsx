import { useState } from 'react';
import { View, Text, Picker, Textarea, Button } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { nextPermissionMode, type PermissionMode } from '@1agents/core/protocol/permission';

const ROLE_OPTIONS = [
  { value: 'general', label: '通用' },
  { value: 'pm', label: '项目经理' },
];

const AGENT_OPTIONS = [
  { value: 'claudecode', label: 'Claude' },
  { value: 'codex', label: 'Codex' },
  { value: 'cursor', label: 'Cursor' },
];

export interface ComposerProps {
  isCreation?: boolean;
  onSend: (text: string) => void | Promise<void>;
  disabled?: boolean;
  creating?: boolean;

  // Permission mode
  permissionMode: PermissionMode;
  onPermissionModeChange: (mode: PermissionMode) => void;

  // Creation-only props
  role?: 'general' | 'pm';
  onRoleChange?: (role: 'general' | 'pm') => void;
  agentType?: string;
  onAgentTypeChange?: (agent: string) => void;
  mode?: 'chat' | 'terminal';
  onModeChange?: (mode: 'chat' | 'terminal') => void;
}

export function Composer({
  isCreation = false,
  onSend,
  disabled = false,
  creating = false,
  permissionMode,
  onPermissionModeChange,
  role = 'general',
  onRoleChange,
  agentType = 'claudecode',
  onAgentTypeChange,
  mode = 'chat',
  onModeChange,
}: ComposerProps) {
  const [value, setValue] = useState('');

  const handleSend = () => {
    if (!value.trim() || disabled || creating) return;
    onSend(value);
    setValue('');
  };

  const cyclePermissionMode = () => {
    const nextMode = nextPermissionMode(permissionMode);
    onPermissionModeChange(nextMode);
    let label = '读取放行';
    if (nextMode === 'auto') label = '智能放行';
    else if (nextMode === 'approve-all') label = '全部放行';
    else if (nextMode === 'deny-all') label = '全部拒绝';
    Taro.showToast({ title: `权限模式已切换至: ${label}`, icon: 'none' });
  };

  return (
    <View className="new-chat-composer-group">
      <View className="new-chat-input-wrapper">
        <Textarea
          className="new-chat-textarea"
          placeholder="输入问题，@ 提及，/ 操作"
          value={value}
          onInput={e => setValue(e.detail.value)}
          maxlength={2000}
          autoHeight
          disabled={disabled}
        />
        <View className="new-chat-actions-row">
          <View className="actions-primary">
            {/* Role picker (Creation only) */}
            {isCreation ? (
              <Picker
                mode="selector"
                range={ROLE_OPTIONS}
                rangeKey="label"
                onChange={e => {
                  const opt = ROLE_OPTIONS[Number(e.detail.value)];
                  if (opt && onRoleChange) onRoleChange(opt.value as 'general' | 'pm');
                }}
              >
                <View className="role-picker-trigger">
                  <Text className="role-name">{role === 'pm' ? '项目经理' : '通用'}</Text>
                  <Text className="chevron">▾</Text>
                </View>
              </Picker>
            ) : (
              // Display current role as static label or hide. Let's show it as a clean badge!
              <View className="role-badge">
                {role === 'pm' ? '项目经理' : '通用'}
              </View>
            )}

            {/* Permission Mode button */}
            <View 
              className="permission-mode-btn"
              onClick={cyclePermissionMode}
            >
              <Text className="btn-emoji">🛡️</Text>
            </View>
          </View>

          <View className="actions-right">
            <View className="action-btn-circle plus-btn">
              <Text className="btn-icon">+</Text>
            </View>
            <View className="action-btn-circle mic-btn">
              <Text className="btn-emoji">🎤</Text>
            </View>
            <Button 
              className={`action-btn-circle send-btn ${!value.trim() ? 'disabled' : ''}`}
              onClick={handleSend}
              disabled={!value.trim() || disabled || creating}
            >
              {creating ? (
                <Text className="btn-icon">⏳</Text>
              ) : (
                <Text className="btn-icon">➔</Text>
              )}
            </Button>
          </View>
        </View>
      </View>

      {/* Bottom outer bar (Creation only) */}
      {isCreation && (
        <View className="new-chat-outer-bar">
          <View className="actions-secondary">
            {/* Mode toggle: chat vs terminal */}
            <View className="new-chat-mode-switch">
              <View 
                className={`mode-switch-btn ${mode === 'chat' ? 'active' : ''}`}
                onClick={() => onModeChange && onModeChange('chat')}
              >
                <Text className="btn-emoji">💬</Text>
              </View>
              <View 
                className={`mode-switch-btn ${mode === 'terminal' ? 'active' : ''}`}
                onClick={() => onModeChange && onModeChange('terminal')}
              >
                <Text className="btn-text">>_</Text>
              </View>
            </View>

            {/* Assistant picker */}
            <Picker
              mode="selector"
              range={AGENT_OPTIONS}
              rangeKey="label"
              onChange={e => {
                const opt = AGENT_OPTIONS[Number(e.detail.value)];
                if (opt && onAgentTypeChange) onAgentTypeChange(opt.value);
              }}
            >
              <View className="agent-picker-trigger">
                <Text className="agent-name">
                  {AGENT_OPTIONS.find(o => o.value === agentType)?.label || agentType}
                </Text>
                <Text className="chevron">▾</Text>
              </View>
            </Picker>
          </View>
        </View>
      )}
    </View>
  );
}
