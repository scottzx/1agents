# @1agents/chat-ui

Conversation and chat timeline rendering components for 1Agents, cross-agent session inspection, and workbench embeds.

## Features

- **High-Fidelity Chat View**: Rich rendering for User, Assistant, Thinking, Tool Groups, and Folded Historical Turns.
- **Syntax Highlighting**: Built-in `highlight.js` with auto language detection and one-click code copying.
- **Git Diffs**: Inline code diffs for tool executions via `ToolDiffView`.
- **Zero Configuration**: Includes complete styles and auto-injects CSS on mount.
- **Framework & DOM Friendly**: Use directly with Preact or mount onto any vanilla DOM container via `mountConversation(container, options)`.

## Usage

```ts
import { mountConversation, turnsToChatItems } from '@1agents/chat-ui';

// Mount directly onto any DOM element
const chat = mountConversation(document.getElementById('chat-container'), {
  turns: sessionData.turns,
  cwd: '/path/to/workspace',
  lang: 'zh-CN',
  onOpenFile: (path) => console.log('Open file:', path),
  onToast: (msg) => console.log('Toast:', msg),
});

// Update or unmount
chat.update({ turns: updatedTurns });
chat.unmount();
```
