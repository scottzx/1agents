import assert from 'node:assert/strict';
import test from 'node:test';

import type { FeatureCatalog, FeatureNode } from '@1agents/core/types/featureCatalog';
import {
    buildFeatureTree,
    deliveryModulePathsByItem,
    featureModulePathsForItem,
    filterFeatureTree,
    flattenFeatureTree,
    formatFeatureError,
    linkedFeatureEntries,
    MAX_FEATURE_MODULE_DEPTH,
    resolveFeatureDrop,
    siblingNodes,
    validFeatureParents,
} from './featureCatalogModel';
import { groupValue } from './gridConfig';

function node(id: string, title: string, kind: FeatureNode['kind'], parentId = '', position = 0): FeatureNode {
    return {
        id,
        title,
        kind,
        parentId: parentId || undefined,
        position,
        createdAt: `2026-01-01T00:00:0${position}Z`,
        updatedAt: '2026-01-01T00:00:00Z',
    };
}

const nodes = [
    node('root', '用户与权限', 'module'),
    node('account', '账号', 'module', 'root'),
    node('login', '登录', 'module', 'account'),
    node('password', '密码登录', 'feature', 'login', 1),
    node('sms', '短信登录', 'feature', 'login', 0),
    node('profile', '个人资料', 'feature', 'root', 1),
];

test('builds complete recursive paths and keeps server sibling order', () => {
    const flat = flattenFeatureTree(buildFeatureTree(nodes));
    const login = flat.find(entry => entry.node.id === 'login');
    const sms = flat.find(entry => entry.node.id === 'sms');
    const password = flat.find(entry => entry.node.id === 'password');

    assert.deepEqual(login?.path, ['用户与权限', '账号', '登录']);
    assert.equal(login?.moduleDepth, 3);
    assert.deepEqual(sms?.path, ['用户与权限', '账号', '登录', '短信登录']);
    assert.ok((flat.indexOf(sms!) ?? -1) < (flat.indexOf(password!) ?? -1));
});

test('filters a feature tree without losing the ancestors that provide context', () => {
    const filtered = filterFeatureTree(buildFeatureTree(nodes), entry => entry.node.id === 'sms');

    assert.deepEqual(
        flattenFeatureTree(filtered).map(entry => entry.node.id),
        ['root', 'account', 'login', 'sms']
    );
    assert.deepEqual(flattenFeatureTree(filtered).at(-1)?.path, ['用户与权限', '账号', '登录', '短信登录']);
});

test('features can use any module parent but can never become parents', () => {
    assert.deepEqual(
        validFeatureParents('feature', nodes).map(parent => parent.id),
        ['root', 'account', 'login']
    );
    assert.equal(
        validFeatureParents('feature', nodes).some(parent => parent.id === 'password'),
        false
    );
});

test('module moves exclude descendants and placements deeper than level nine', () => {
    const root = nodes.find(item => item.id === 'root')!;
    const account = nodes.find(item => item.id === 'account')!;
    const login = nodes.find(item => item.id === 'login')!;

    assert.deepEqual(validFeatureParents('module', nodes, root), []);
    assert.deepEqual(
        validFeatureParents('module', nodes, account).map(parent => parent.id),
        ['root']
    );
    assert.deepEqual(
        validFeatureParents('module', nodes, login).map(parent => parent.id),
        ['root', 'account']
    );
});

test('allows nine module levels, keeps feature leaves at level nine, and rejects deeper module parents', () => {
    const deepNodes: FeatureNode[] = [];
    for (let depth = 1; depth <= MAX_FEATURE_MODULE_DEPTH; depth++) {
        deepNodes.push(node(`m${depth}`, `模块 ${depth}`, 'module', depth === 1 ? '' : `m${depth - 1}`));
    }
    deepNodes.push(node('leaf', '功能点', 'feature', 'm9'));

    assert.equal(
        flattenFeatureTree(buildFeatureTree(deepNodes)).find(entry => entry.node.id === 'leaf')?.moduleDepth,
        9
    );
    assert.equal(
        validFeatureParents('feature', deepNodes).some(parent => parent.id === 'm9'),
        true
    );
    assert.equal(
        validFeatureParents('module', deepNodes).some(parent => parent.id === 'm9'),
        false
    );
});

test('resolves before, inside, after, and root drops without same-parent off-by-one errors', () => {
    const dragNodes = [
        node('root-a', 'A', 'module', '', 0),
        node('root-b', 'B', 'module', '', 1),
        node('child-a', 'A1', 'module', 'root-a', 0),
        node('feature-a', '功能 A', 'feature', 'root-a', 1),
        node('feature-b', '功能 B', 'feature', 'root-a', 2),
    ];

    assert.deepEqual(resolveFeatureDrop(dragNodes, 'root-a', { targetId: 'root-b', placement: 'after' }), {
        parentId: undefined,
        position: 1,
    });
    assert.deepEqual(resolveFeatureDrop(dragNodes, 'root-a', { targetId: 'root-b', placement: 'inside' }), {
        parentId: 'root-b',
        position: 0,
    });
    assert.deepEqual(resolveFeatureDrop(dragNodes, 'feature-a', { targetId: 'feature-b', placement: 'after' }), {
        parentId: 'root-a',
        position: 2,
    });
    assert.deepEqual(resolveFeatureDrop(dragNodes, 'child-a', { placement: 'root' }), {
        parentId: undefined,
        position: 2,
    });
    assert.equal(resolveFeatureDrop(dragNodes, 'feature-a', { placement: 'root' }), null);
    assert.equal(resolveFeatureDrop(dragNodes, 'root-a', { targetId: 'child-a', placement: 'inside' }), null);
});

test('rejects a drop that would place a module subtree below level nine', () => {
    const deepNodes: FeatureNode[] = [
        node('branch', '分支', 'module'),
        node('branch-child', '分支子模块', 'module', 'branch'),
    ];
    for (let depth = 1; depth <= 8; depth++) {
        deepNodes.push(node(`target-${depth}`, `目标 ${depth}`, 'module', depth === 1 ? '' : `target-${depth - 1}`));
    }

    assert.equal(resolveFeatureDrop(deepNodes, 'branch', { targetId: 'target-8', placement: 'inside' }), null);
    assert.deepEqual(resolveFeatureDrop(deepNodes, 'branch-child', { targetId: 'target-8', placement: 'inside' }), {
        parentId: 'target-8',
        position: 0,
    });
});

test('sibling ordering and backend validation errors are deterministic', () => {
    assert.deepEqual(
        siblingNodes(nodes, 'login').map(item => item.id),
        ['sms', 'password']
    );
    assert.equal(
        formatFeatureError(new Error('meta: feature module depth exceeds nine')),
        '模块最多支持九级，请选择更高层级的父模块。'
    );
    assert.equal(formatFeatureError(new Error('has_children')), '该模块仍有子节点，请先移动或删除子节点。');
    assert.equal(
        formatFeatureError(new Error('meta: invalid project item type for feature relation')),
        '关联类型不匹配：来源仅支持需求/缺陷，交付仅支持任务。'
    );
    assert.equal(
        formatFeatureError(new Error('meta: feature target must be a semantic version milestone')),
        '目标版本必须是当前项目的语义化版本，不能选择 legacy 里程碑。'
    );
});

test('resolves reverse traceability and every complete module path for multi-feature tasks', () => {
    const secondRoot = node('billing', '商业化', 'module', '', 1);
    const invoice = node('invoice', '发票', 'feature', 'billing');
    const catalog: FeatureCatalog = {
        nodes: [...nodes, secondRoot, invoice],
        links: [
            { featureId: 'sms', itemId: 'task-1', relation: 'delivery', createdAt: '' },
            { featureId: 'password', itemId: 'task-1', relation: 'delivery', createdAt: '' },
            { featureId: 'invoice', itemId: 'task-1', relation: 'delivery', createdAt: '' },
            { featureId: 'sms', itemId: 'requirement-1', relation: 'source', createdAt: '' },
        ],
    };

    assert.deepEqual(featureModulePathsForItem(catalog, 'task-1', 'delivery'), ['商业化', '用户与权限 / 账号 / 登录']);
    assert.deepEqual(
        linkedFeatureEntries(catalog, 'requirement-1', 'source').map(entry => entry.path.join(' / ')),
        ['用户与权限 / 账号 / 登录 / 短信登录']
    );
    assert.deepEqual(featureModulePathsForItem(catalog, 'unlinked', 'delivery'), []);
    assert.deepEqual(deliveryModulePathsByItem(catalog).get('task-1'), ['商业化', '用户与权限 / 账号 / 登录']);
    assert.deepEqual(
        groupValue(
            { id: 'task-1' } as never,
            'feature',
            'zh-CN',
            featureModulePathsForItem(catalog, 'task-1', 'delivery')
        ),
        ['商业化', '用户与权限 / 账号 / 登录']
    );
    assert.equal(groupValue({ id: 'unlinked' } as never, 'feature', 'zh-CN', []), '未归入功能蓝图');
});
