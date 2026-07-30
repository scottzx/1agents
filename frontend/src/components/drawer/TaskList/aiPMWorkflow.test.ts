import assert from 'node:assert/strict';
import test from 'node:test';

import { buildFeatureBreakdownPrompt, buildFeatureCatalogGeneratePrompt } from './aiPMWorkflow';

test('catalog generation prompt gates writes and preserves an existing tree', () => {
    const prompt = buildFeatureCatalogGeneratePrompt(12);

    assert.match(prompt, /featureCatalogEnabled/);
    assert.match(prompt, /绝不创建隐藏蓝图数据/);
    assert.match(prompt, /读取完整现有树/);
    assert.match(prompt, /明确确认/);
    assert.match(prompt, /禁止无确认整体覆盖/);
    assert.match(prompt, /最多九级模块和功能点/);
    assert.match(prompt, /结构化变更摘要/);
});

test('feature breakdown prompt requires source, delivery, and inherited version', () => {
    const prompt = buildFeatureBreakdownPrompt({
        featureId: 'feature-1',
        title: '验证码登录',
        path: '用户与权限 / 用户认证 / 登录 / 验证码登录',
        sources: ['#12 登录需求 [requirement-1]'],
        targetVersion: '0.4.0',
    });

    assert.match(prompt, /feature-1/);
    assert.match(prompt, /顶层 requirement\/bug/);
    assert.match(prompt, /acceptanceCriteria/);
    assert.match(prompt, /links/);
    assert.match(prompt, /delivery 关联/);
    assert.match(prompt, /继承功能点目标版本/);
    assert.match(prompt, /结构化变更摘要/);
});
