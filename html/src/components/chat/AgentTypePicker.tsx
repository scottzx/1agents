import { h, Component } from 'preact';
import { AGENT_TYPES, AGENT_TYPE_LABELS, type AgentType } from '../types';
import { pickableAgents } from '../../stores/agentCatalogStore';

interface AgentTypePickerProps {
    value: AgentType;
    onChange: (val: AgentType) => void;
    disabled?: boolean;
}

/**
 * A simple <select> for picking an agent type. Reused by the workspace
 * modal (defaultAgent) and the session create modal.
 *
 * Options come from the live agent catalog (installed agents only). Before
 * the catalog loads — or if the probe failed — it falls back to the static
 * AGENT_TYPES list so the picker is never empty. The current value is always
 * kept selectable even when uninstalled, so an existing config still renders.
 */
export class AgentTypePicker extends Component<AgentTypePickerProps> {
    render() {
        const { value, onChange, disabled } = this.props;

        const pickable = pickableAgents.value;
        const options: { type: AgentType; label: string }[] = pickable.length
            ? pickable.map(a => ({ type: a.type, label: a.label }))
            : AGENT_TYPES.map(t => ({ type: t, label: AGENT_TYPE_LABELS[t] ?? t }));

        // Keep the selected value present even if it isn't installed.
        if (value && !options.some(o => o.type === value)) {
            options.unshift({ type: value, label: AGENT_TYPE_LABELS[value] ?? value });
        }

        return (
            <select
                class="agent-type-picker"
                value={value}
                disabled={disabled}
                onChange={(e: Event) => onChange((e.target as HTMLSelectElement).value as AgentType)}
            >
                {options.map(o => (
                    <option key={o.type} value={o.type}>
                        {o.label}
                    </option>
                ))}
            </select>
        );
    }
}
