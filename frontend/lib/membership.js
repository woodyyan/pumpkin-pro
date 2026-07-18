/**
 * 会员体系（预发布）配置与纯逻辑。
 *
 * 本期为「展示-only」预发布：不接支付、无后端接口、无真实会员状态。
 * 所有价格 / 权益 / 规则文案集中在本文件，收集反馈后调整只需改配置。
 * 后续接真实收费时，仅需替换：
 *   1. MEMBERSHIP_PRELAUNCH 置为 false 并接入真实会员状态来源
 *   2. 开通按钮行为（当前为占位弹层）
 */

// 预发布模式：强制所有用户按「非会员」形态展示
export const MEMBERSHIP_PRELAUNCH = true

export const MEMBERSHIP_PATH = '/membership'

// 反馈入口：设置页「反馈与建议」板块
export const MEMBERSHIP_FEEDBACK_PATH = '/settings#feedback'

export const MEMBERSHIP_LAUNCH_NOTE = '具体价格与规则以正式上线公告为准。'

export const MEMBERSHIP_PLANS = [
  {
    key: 'monthly',
    name: '月度会员',
    price: 39,
    unit: '月',
    priceLabel: '¥39/月',
    description: '按月订阅，灵活体验完整会员权益',
    highlight: false,
  },
  {
    key: 'yearly',
    name: '年度会员',
    price: 390,
    unit: '年',
    priceLabel: '¥390/年',
    badge: '约 8.3 折 · 买 10 个月送 2 个月',
    description: '一次订阅全年无忧，折合每月仅 ¥32.5',
    highlight: true,
  },
]

// 免费 vs 会员 对比表（会员权益全部在此呈现）
export const MEMBERSHIP_COMPARE_ROWS = [
  { feature: 'AI 研报', free: '按份购买', member: '每月 5 份 AI 研报额度，用完后可按份购买' },
  { feature: 'AI 深度投研', free: '每日少量免费额度（每日 0 点北京时间重置）', member: '更完整的个股诊断、深度追问与研究记录' },
  { feature: '市场行情与机会榜', free: '基础行情与有限榜单', member: '完整榜单与历史趋势' },
  { feature: '自选股与选股器', free: '少量自选股、基础条件', member: '更高上限、高级因子、筛选器保存' },
  { feature: '策略回测', free: '示例策略与摘要结果', member: '自定义参数、全量指标、结果导出' },
  { feature: '组合与风险管理', free: '组合模板', member: '持仓归因、风险诊断、预警提醒' },
  { feature: '信号推送', free: '低频站内提醒', member: '更高频提醒与 Webhook 推送' },
]

export const MEMBERSHIP_FREE_QUOTA_NOTE = '免费额度每日 0 点（北京时间）重置。'

export const MEMBERSHIP_FAQS = [
  {
    question: '会员包含 AI 研报吗？',
    answer: '会员每月拥有 5 份 AI 研报额度；用完后可按份单独购买。',
  },
  {
    question: '年付会员怎么算？',
    answer: '年度会员 ¥390/年，相当于约 8.3 折（买 10 个月送 2 个月），折合每月约 ¥32.5。',
  },
  {
    question: '什么时候开始正式收费？',
    answer: '本期为会员体系预发布，仅展示权益并收集反馈，暂不开通支付。正式上线时间请关注站内公告。',
  },
]

/**
 * 解析顶部账户区的会员入口形态。
 *
 * 预发布期间（MEMBERSHIP_PRELAUNCH=true）强制按非会员处理；
 * 后续接入真实会员状态后，传入真实 isMember 即可。
 *
 * @returns {'loading'|'guest'|'non-member'|'member'}
 */
export function resolveMembershipEntryState({ ready = true, isLoggedIn = false, isMember = false } = {}) {
  if (!ready) return 'loading'
  if (!isLoggedIn) return 'guest'
  if (MEMBERSHIP_PRELAUNCH) return 'non-member'
  return isMember ? 'member' : 'non-member'
}

/** 已登录会员的下拉菜单文案；非会员为「会员中心」 */
export function buildMembershipMenuLabel(state, { expiresAt = '' } = {}) {
  if (state === 'member' && expiresAt) {
    return `会员中心 · 有效期至 ${expiresAt}`
  }
  return '会员中心'
}
