/**
 * Personal Shell cross-shell work aggregation service (task #329). Typed
 * wrapper around GET /api/agent/personal/aggregate, routed through apiFetch so
 * it works over the relay / from the mini-app as well as same-origin web.
 */

import { apiFetch } from './apiClient';
import type { PersonalAggregateQuery, PersonalAggregateResponse } from '../types/workcase';

const q = encodeURIComponent;

/** Builds the query string for the aggregate endpoint from the given params. */
export function buildAggregateQuery(params: PersonalAggregateQuery): string {
    const parts: string[] = [];
    if (params.bucket && params.bucket !== 'all') parts.push(`bucket=${q(params.bucket)}`);
    if (params.workspace) parts.push(`workspace=${q(params.workspace)}`);
    if (params.case) parts.push(`case=${q(params.case)}`);
    if (params.status) parts.push(`status=${q(params.status)}`);
    if (params.sort) parts.push(`sort=${q(params.sort)}`);
    if (params.dir) parts.push(`dir=${q(params.dir)}`);
    if (params.limit !== undefined) parts.push(`limit=${params.limit}`);
    if (params.offset !== undefined) parts.push(`offset=${params.offset}`);
    if (params.actor) parts.push(`actor=${q(params.actor)}`);
    return parts.length ? `?${parts.join('&')}` : '';
}

export const personalAggregateService = {
    /** Fetch the Personal Shell cross-shell work aggregate. */
    async fetch(params: PersonalAggregateQuery = {}): Promise<PersonalAggregateResponse> {
        const res = await apiFetch(`/agent/personal/aggregate${buildAggregateQuery(params)}`);
        if (!res.ok) throw new Error(`Failed to load aggregate: ${res.statusText}`);
        return (await res.json()) as PersonalAggregateResponse;
    },
};
