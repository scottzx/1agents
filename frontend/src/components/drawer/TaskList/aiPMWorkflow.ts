export interface FeatureBreakdownPromptContext {
    featureId: string;
    title: string;
    path: string;
    sources: string[];
    targetVersion: string;
}

const STRUCTURED_SUMMARY = [
    '写入完成后重新读取功能蓝图和相关项目项，并给出“结构化变更摘要”：',
    '1. 节点：按完整路径列出新增、修改、移动的模块和功能点；',
    '2. 追溯：逐个功能点列出 source 的顶层需求/缺陷，以及 delivery 的任务编号；',
    '3. 版本：列出功能点目标版本及每个新任务实际继承的版本；',
    '4. 未变更/待确认：明确说明保留的既有节点和仍需用户决定的事项。',
].join('\n');

export function buildFeatureCatalogGeneratePrompt(existingNodeCount: number): string {
    const existingTreeInstruction =
        existingNodeCount > 0
            ? `界面当前显示 ${existingNodeCount} 个蓝图节点。必须先读取完整现有树，只做增量维护；先展示拟议差异并获得我的明确确认，禁止无确认整体覆盖、批量删除或重建既有蓝图。`
            : '界面当前为空。先读取需求池并和我确认范围；确认后可用事务化 batch 一次提交最多九级模块和功能点。';

    return [
        '请使用 pm Skill 与我一起生成或维护当前项目的功能蓝图。',
        '先读取 .1agents/project_config.json 的 featureCatalogEnabled。若不是 true，停止蓝图写入并说明应改走轻量“需求 → 任务”流程，绝不创建隐藏蓝图数据。',
        '随后读取项目需求/缺陷、里程碑和完整功能蓝图，不能把界面摘要当作完整数据。',
        existingTreeInstruction,
        '每个功能点都要关联已确认的顶层 requirement/bug 作为 source；缺少来源或目标版本时先向我澄清。',
        '写入树时优先使用 feature-catalog batch/clientRef/parentRef，让最多九级模块和功能点通过一次原子提交落库；任一校验失败时不要改成逐条写入来留下半棵树。',
        STRUCTURED_SUMMARY,
    ].join('\n\n');
}

export function buildFeatureBreakdownPrompt(context: FeatureBreakdownPromptContext): string {
    const sources = context.sources.length > 0 ? context.sources.join('；') : '尚未关联';

    return [
        '请使用 pm Skill 将当前功能点拆解为可执行任务。',
        '先读取 .1agents/project_config.json 的 featureCatalogEnabled，并重新读取完整功能蓝图和项目项。若开关不是 true，停止蓝图写入并说明应改走轻量“需求 → 任务”流程。',
        `功能点：${context.title}\n完整路径：${context.path}\nfeatureId：${context.featureId}\n当前 source：${sources}\n目标版本：${context.targetVersion}`,
        '先确认该功能点归口的顶层 requirement/bug；若没有唯一且明确的来源，先问我，不要创建无归口任务。',
        '创建的每个 task 都必须有可检验的 acceptanceCriteria，并同时：在 description 中引用顶层 #需求编号或通过 links 建立 relates 归口；传入上述 featureId，让服务端自动建立 delivery 关联并继承功能点目标版本；按实际执行顺序设置 dependsOn。不要另传一个与功能点冲突的版本。',
        '只拆解这个功能点，不要整体重写现有蓝图。若认为需要移动、删除或重命名既有节点，先展示差异并等待我的明确确认。',
        STRUCTURED_SUMMARY,
    ].join('\n\n');
}
