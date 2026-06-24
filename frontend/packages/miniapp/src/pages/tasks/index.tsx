// 任务 sub-page — read-only task board for the first workspace, over @1agents/core.
// Cards are derived from the shared core taskCardVM (same status→tone logic the
// web board uses); only tone→Tag tone and key→label are host-local. The terminal
// stays off weapp; this is one of the light surfaces. No create/edit yet.
import { View, Text } from '@tarojs/components';
import { useEffect, useState } from 'react';

import { workspaceService } from '@1agents/core/services/workspaceService';
import { taskService } from '@1agents/core/services/taskService';
import { taskCardVM, type Tone } from '@1agents/core/view/taskCard';
import type { Task } from '@1agents/core/types/task';

import { Screen } from '../../components/Screen';
import { Card } from '../../components/ui/Card';
import { Section } from '../../components/ui/Section';
import { Tag, type TagTone } from '../../components/ui/Tag';
import { EmptyState } from '../../components/ui/EmptyState';
import { Loading } from '../../components/ui/Loading';
import { useT } from '../../hooks/useUI';
import './index.scss';

// The core view-model speaks the full SCSS tone vocabulary; the mini-program's
// Tag only has accent/success/danger/warning/muted (no orange/purple token), so
// collapse the two extras to their nearest Tag tone.
const TAG_TONE: Record<Tone, TagTone> = {
  accent: 'accent',
  success: 'success',
  danger: 'danger',
  warning: 'warning',
  orange: 'warning',
  purple: 'accent',
  muted: 'muted',
};

export default function Tasks() {
  const t = useT();
  const [tasks, setTasks] = useState<Task[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setInterval> | undefined;

    const load = async (wsId: string) => {
      try {
        const list = await taskService.list(wsId);
        if (!cancelled) {
          setTasks(list);
          setError('');
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    (async () => {
      try {
        const wss = await workspaceService.list();
        const ws = wss[0];
        if (!ws) {
          setError(t('tasks.noWorkspace'));
          setLoading(false);
          return;
        }
        await load(ws.id);
        // Poll like the web board so status changes surface without a manual refresh.
        timer = setInterval(() => void load(ws.id), 5000);
      } catch (e) {
        if (!cancelled) {
          setError(String(e));
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
      if (timer) clearInterval(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // AI suggestions are held out of the board until 采纳/忽略 (no such UI here yet).
  const cards = tasks.filter((task) => task.source !== 'agent-suggested').map(taskCardVM);

  return (
    <Screen titleKey="tasks.title">
      <View className="tasks-content">
        <View className="tasks-head">
          <Text className="tasks-head__title">{t('tasks.title')}</Text>
          <Text className="tasks-head__sub">{t('tasks.subtitle')}</Text>
        </View>

        {loading ? (
          <Loading text={t('tasks.loading')} />
        ) : error ? (
          <EmptyState icon="⚠️" title={t('tasks.error')} desc={error} />
        ) : cards.length === 0 ? (
          <EmptyState icon="🗂️" title={t('tasks.empty')} />
        ) : (
          <Section title={t('tasks.section')}>
            {cards.map((c) => (
              <Card key={c.id} className={`task-card tone--${c.statusTone}${c.isTerminal ? ' task-card--done' : ''}`}>
                <View className="task-card__head">
                  {c.numberLabel ? <Text className="task-card__num">{c.numberLabel}</Text> : null}
                  <Text className="task-card__title">{c.title}</Text>
                </View>
                <View className="task-card__tags">
                  <Tag text={t(`task.status.${c.status}`)} tone={TAG_TONE[c.statusTone]} />
                  <Tag text={t(`task.priority.${c.priority}`)} tone={TAG_TONE[c.priorityTone]} />
                  <Tag text={t(`task.type.${c.type}`)} tone="muted" />
                  {c.assignee ? <Tag text={c.assignee} tone="muted" /> : null}
                </View>
              </Card>
            ))}
          </Section>
        )}
      </View>
    </Screen>
  );
}
