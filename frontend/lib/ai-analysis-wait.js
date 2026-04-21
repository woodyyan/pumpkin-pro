export const AI_ANALYSIS_WAIT_STAGES = [
  {
    key: 'prepare',
    kicker: '阶段 1/5',
    startSec: 0,
    endSec: 4,
    progressStart: 8,
    progressEnd: 12,
    getTitle: () => '正在准备分析上下文',
    getHint: () => '先检查行情快照并整理本次分析输入',
  },
  {
    key: 'market',
    kicker: '阶段 2/5',
    startSec: 5,
    endSec: 12,
    progressStart: 12,
    progressEnd: 28,
    getTitle: () => '正在聚合实时行情与技术指标',
    getHint: () => '会优先整理价格、量能、均线与动量信号',
  },
  {
    key: 'fundamentals',
    kicker: '阶段 3/5',
    startSec: 13,
    endSec: 24,
    progressStart: 28,
    progressEnd: 48,
    getTitle: () => '正在整理基础面与市场环境',
    getHint: () => '同时补充估值、财务摘要与大盘背景',
  },
  {
    key: 'context',
    kicker: '阶段 4/5',
    startSec: 25,
    endSec: 40,
    progressStart: 48,
    progressEnd: 72,
    getTitle: ({ hasPosition }) => hasPosition
      ? '正在整理市场环境与持仓上下文'
      : '正在整理市场环境与风险偏好',
    getHint: ({ hasPosition }) => hasPosition
      ? '会把当前持仓盈亏一起纳入判断'
      : '即使没有持仓，也会从空仓视角给出建议',
  },
  {
    key: 'conclusion',
    kicker: '阶段 5/5',
    startSec: 41,
    endSec: 55,
    progressStart: 72,
    progressEnd: 88,
    getTitle: () => '正在生成结论与风险提示',
    getHint: () => '马上进入最终结果整理',
  },
  {
    key: 'slow',
    kicker: '耗时偏长',
    startSec: 56,
    endSec: 80,
    progressStart: 88,
    progressEnd: 92,
    getTitle: () => '本次分析耗时略长，仍在整理最终结果',
    getHint: () => '你可以留在当前页等待，结果出来后会自动展示',
  },
]

const AI_ANALYSIS_WAIT_STEP_DEFS = [
  {
    key: 'market',
    label: '实时行情',
    startSec: 0,
    doneSec: 6,
    getPendingText: () => '等待开始',
    getActiveText: () => '正在读取最新快照',
    getDoneText: () => '已完成读取',
  },
  {
    key: 'technical',
    label: '技术指标',
    startSec: 6,
    doneSec: 12,
    getPendingText: () => '等待上一阶段',
    getActiveText: () => '正在整理均线与动量',
    getDoneText: () => '已完成整理',
  },
  {
    key: 'fundamentals',
    label: '基础面',
    startSec: 12,
    doneSec: 24,
    getPendingText: () => '等待上一阶段',
    getActiveText: () => '正在整理估值与财务摘要',
    getDoneText: () => '已完成整理',
  },
  {
    key: 'market_overview',
    label: '大盘环境',
    startSec: 24,
    doneSec: 34,
    getPendingText: () => '等待上一阶段',
    getActiveText: () => '正在补充指数与市场背景',
    getDoneText: () => '已完成整理',
  },
  {
    key: 'portfolio',
    label: '持仓上下文',
    startSec: 34,
    doneSec: 44,
    getPendingText: ({ hasPosition }) => hasPosition ? '等待上一阶段' : '如无持仓将按空仓视角评估',
    getActiveText: ({ hasPosition }) => hasPosition ? '正在结合你的持仓盈亏' : '正在按空仓视角评估',
    getDoneText: ({ hasPosition }) => hasPosition ? '已纳入持仓信息' : '已按空仓视角评估',
  },
  {
    key: 'conclusion',
    label: 'AI 结论',
    startSec: 44,
    doneSec: Number.POSITIVE_INFINITY,
    getPendingText: () => '等待上一阶段',
    getActiveText: () => '正在生成结论与风险提示',
    getDoneText: () => '已完成生成',
  },
]

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value))
}

function lerp(start, end, ratio) {
  return start + (end - start) * ratio
}

function resolveStage(elapsedSec) {
  return AI_ANALYSIS_WAIT_STAGES.find((stage) => elapsedSec <= stage.endSec) || AI_ANALYSIS_WAIT_STAGES[AI_ANALYSIS_WAIT_STAGES.length - 1]
}

function computeStageProgress(elapsedSec, stage) {
  const span = Math.max(1, stage.endSec - stage.startSec)
  const ratio = clamp((elapsedSec - stage.startSec) / span, 0, 1)
  return Math.round(lerp(stage.progressStart, stage.progressEnd, ratio))
}

export function deriveAIAnalysisWaitState(elapsedSec, options = {}) {
  const hasPosition = Boolean(options.hasPosition)
  const safeElapsed = clamp(Number.isFinite(Number(elapsedSec)) ? Number(elapsedSec) : 0, 0, 3600)
  const stage = resolveStage(safeElapsed)
  const progress = computeStageProgress(safeElapsed, stage)

  const steps = AI_ANALYSIS_WAIT_STEP_DEFS.map((step) => {
    let status = 'pending'
    if (safeElapsed >= step.doneSec && Number.isFinite(step.doneSec)) {
      status = 'done'
    } else if (safeElapsed >= step.startSec) {
      status = 'active'
    }

    const textGetter = status === 'done'
      ? step.getDoneText
      : status === 'active'
        ? step.getActiveText
        : step.getPendingText

    return {
      key: step.key,
      label: step.label,
      status,
      description: textGetter({ hasPosition }),
    }
  })

  return {
    elapsedSec: safeElapsed,
    progress,
    stage: {
      key: stage.key,
      kicker: stage.kicker,
      title: stage.getTitle({ hasPosition }),
      hint: stage.getHint({ hasPosition }),
    },
    steps,
  }
}
