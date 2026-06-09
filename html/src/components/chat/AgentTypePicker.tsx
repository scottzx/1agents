import { h, Component } from 'preact';
import { AGENT_TYPES, AGENT_TYPE_LABELS, type AgentType } from '../types';

interface AgentTypePickerProps {
    value: AgentType;
    onChange: (val: AgentType) => void;
    disabled?: boolean;
}

/**
 * A simple <select> for picking an agent type. Reused by the workspace
 * modal (defaultAgent) and the session create modal.
 */
export class AgentTypePicker extends Component<AgentTypePickerProps> {
    render() {
        const { value, onChange, disabled } = this.props;
        return (
            <select
                class="agent-type-picker"
                value={value}
                disabled={disabled}
                onChange={(e: Event) => onChange((e.target as HTMLSelectElement).value as AgentType)}
            >
                {AGENT_TYPES.map(t => (
                    <option key={t} value={t}>
                        {AGENT_TYPE_LABELS[t] ?? t}
                    </option>
                ))}
            </select>
        );
    }
}
