import Head from 'next/head'
import Link from 'next/link'

export default function Disclaimer() {
  return (
    <>
      <Head><title>免责声明 — 卧龙AI量化交易台</title></Head>
      <div className="max-w-3xl mx-auto pb-16 space-y-8">
        <header className="space-y-2">
          <h1 className="text-3xl font-bold tracking-tight">免责声明</h1>
          <p className="text-sm text-white/45">最后更新日期：2026 年 4 月 2 日 · 生效日期：2026 年 4 月 2 日</p>
        </header>

        <div className="prose prose-invert prose-sm max-w-none space-y-6 text-white/75 leading-7">
          <p>在使用「卧龙AI量化交易台」（以下简称「本平台」）前，请您仔细阅读以下免责声明。使用本平台即表示您已理解并接受以下全部条款。本平台由 Easy Studio Inc. 运营。</p>

          <h2 className="text-lg font-semibold text-white">1. 非投资建议声明</h2>
          <div className="rounded-xl border border-amber-400/20 bg-amber-500/8 px-4 py-3 text-amber-200/90">
            <strong>本平台提供的所有数据、分析、策略回测结果、交易信号及其他内容，均仅供参考和学习研究之用，不构成任何形式的投资建议、投资推荐或投资决策依据。</strong>
          </div>
          <p>本平台不具备证券投资咨询资质，未持有中国证监会颁发的证券投资咨询业务许可证。本平台的任何内容均不应被理解为对任何证券的买入、卖出或持有的推荐。</p>

          <h2 className="text-lg font-semibold text-white">2. 投资风险提示</h2>
          <div className="rounded-xl border border-rose-400/20 bg-rose-500/8 px-4 py-3 text-rose-200/90">
            <strong>股票市场存在风险，投资需谨慎。过往的回测业绩和策略表现不代表未来收益，任何策略都可能产生亏损。</strong>
          </div>
          <p>2.1 您基于本平台的信息所做出的任何投资决策，均为您的<strong>个人行为</strong>，由此产生的盈亏和风险<strong>完全由您自行承担</strong>。</p>
          <p>2.2 本平台不对您因使用本平台数据或信号所遭受的任何直接或间接损失承担责任，包括但不限于：</p>
          <ul className="list-disc pl-5 space-y-1">
            <li>因行情数据延迟或不准确导致的损失</li>
            <li>因策略信号触发后的实际交易产生的损失</li>
            <li>因 Webhook 推送延迟、失败或重复导致的损失</li>
            <li>因市场波动、停牌、熔断等不可预见事件导致的损失</li>
          </ul>

          <h2 className="text-lg font-semibold text-white">3. 数据准确性免责</h2>
          <p>3.1 本平台展示的行情数据来自腾讯财经、东方财富等第三方公开接口，本平台<strong>不保证</strong>数据的准确性、完整性、及时性。</p>
          <p>3.2 本平台的 AI 选股功能基于自然语言处理技术，可能存在理解偏差，筛选结果仅供参考。</p>
          <p>3.3 基本面数据来自上市公司公开披露的财务报告，可能存在报告期差异、数据更新延迟等情况。</p>
          <p>3.4 风险机会全景图（四象限模型）基于量化模型计算，模型本身存在局限性，象限分类结果不代表对个股的评级或推荐。</p>

          <h2 className="text-lg font-semibold text-white">4. 信号推送免责</h2>
          <p>4.1 本平台的交易信号仅为基于预设策略参数的<strong>自动化计算结果</strong>，不代表本平台对该交易方向的建议或推荐。</p>
          <p>4.2 信号通过用户自行配置的 Webhook 推送至第三方服务（如企业微信、钉钉等），本平台不保证推送的及时性和可靠性。</p>
          <p>4.3 用户如将信号用于自动化交易系统，需自行评估风险并承担全部后果。本平台<strong>强烈建议</strong>在使用信号前进行人工确认。</p>

          <h2 className="text-lg font-semibold text-white">5. 服务可用性免责</h2>
          <p>5.1 本平台不保证服务的不间断运行。因网络故障、服务器维护、第三方数据源异常等原因导致的服务中断，本平台不承担责任。</p>
          <p>5.2 因不可抗力（包括但不限于自然灾害、政策变化、网络攻击等）导致的服务中断或数据丢失，本平台不承担责任。</p>

          <h2 className="text-lg font-semibold text-white">6. 第三方服务免责</h2>
          <p>本平台可能包含指向第三方网站或服务的链接。对于第三方的内容、隐私政策或做法，本平台不承担任何责任。</p>

          <h2 className="text-lg font-semibold text-white">7. 责任上限</h2>
          <p>在法律允许的最大范围内，本平台因使用或无法使用本服务而产生的全部赔偿责任，不超过您在过去 12 个月内向本平台支付的费用总额（如为免费用户，则上限为人民币 0 元）。</p>

          <h2 className="text-lg font-semibold text-white">8. 适用对象</h2>
          <p>本平台面向具有完全民事行为能力的成年人，且仅供具有一定证券投资知识和风险承受能力的用户使用。如您不具备相应的知识和能力，请勿依赖本平台做出投资决策。</p>
        </div>

        <div className="pt-6 border-t border-white/10 flex gap-4 text-sm text-white/40">
          <Link href="/privacy" className="hover:text-white/70 transition">隐私政策</Link>
          <Link href="/terms" className="hover:text-white/70 transition">用户协议</Link>
        </div>
      </div>
    </>
  )
}
