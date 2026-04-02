import Head from 'next/head'
import Link from 'next/link'

export default function Terms() {
  return (
    <>
      <Head><title>用户协议 — 卧龙AI量化交易台</title></Head>
      <div className="max-w-3xl mx-auto pb-16 space-y-8">
        <header className="space-y-2">
          <h1 className="text-3xl font-bold tracking-tight">用户协议</h1>
          <p className="text-sm text-white/45">最后更新日期：2026 年 4 月 2 日 · 生效日期：2026 年 4 月 2 日</p>
        </header>

        <div className="prose prose-invert prose-sm max-w-none space-y-6 text-white/75 leading-7">
          <p>欢迎使用「卧龙AI量化交易台」（以下简称「本平台」）。本平台由 Easy Studio Inc. 运营。请您在注册和使用前仔细阅读本用户协议（以下简称「本协议」）。注册或使用本平台即表示您已阅读、理解并同意接受本协议的全部条款。</p>

          <h2 className="text-lg font-semibold text-white">1. 服务说明</h2>

          <h3 className="text-base font-medium text-white/90">1.1 本平台提供的服务</h3>
          <p>本平台是一个股票行情分析与量化策略辅助工具，具体包括：</p>
          <ul className="list-disc pl-5 space-y-1">
            <li>股票行情看板（实时/延迟行情数据展示）</li>
            <li>量化策略回测（基于历史数据的策略模拟）</li>
            <li>交易信号评估与 Webhook 推送</li>
            <li>选股筛选与全市场扫描</li>
            <li>技术指标分析（均线、MACD、布林带等）</li>
            <li>基本面数据展示</li>
            <li>持仓记录与盈亏跟踪</li>
            <li>风险机会全景图（四象限模型）</li>
          </ul>

          <h3 className="text-base font-medium text-white/90">1.2 本平台不提供的服务</h3>
          <ul className="list-disc pl-5 space-y-1">
            <li>本平台<strong>不是</strong>证券交易平台，不提供任何证券买卖、下单、撮合等交易功能。</li>
            <li>本平台<strong>不是</strong>证券投资咨询机构，不提供任何形式的投资建议或推荐。</li>
            <li>本平台<strong>不托管</strong>任何用户资金，不涉及资金划转。</li>
          </ul>

          <h2 className="text-lg font-semibold text-white">2. 账户注册与安全</h2>
          <p>2.1 您应使用真实有效的邮箱注册账户，并妥善保管账户密码。</p>
          <p>2.2 您对账户下的所有活动承担责任。如发现账户被未经授权使用，请立即联系我们。</p>
          <p>2.3 我们保留在发现违规行为时暂停或终止账户的权利。</p>

          <h2 className="text-lg font-semibold text-white">3. 用户行为规范</h2>
          <p>使用本平台时，您承诺：</p>
          <p>3.1 <strong>不会</strong>利用本平台从事任何违反法律法规的活动。</p>
          <p>3.2 <strong>不会</strong>通过技术手段（如爬虫、自动化脚本等）对本平台进行恶意请求或攻击。</p>
          <p>3.3 <strong>不会</strong>将本平台提供的数据和分析结果以商业目的批量转售给第三方。</p>
          <p>3.4 <strong>不会</strong>利用本平台的信号推送功能发送任何与证券交易无关的信息。</p>

          <h2 className="text-lg font-semibold text-white">4. 数据来源与准确性</h2>
          <p>4.1 本平台展示的行情数据来自第三方公开数据源，可能存在延迟或偏差。</p>
          <p>4.2 本平台展示的基本面数据来自公开财务报告，数据的准确性和时效性取决于上市公司的信息披露。</p>
          <p>4.3 本平台的量化策略回测基于历史数据，<strong>历史表现不代表未来收益</strong>。</p>
          <p>4.4 本平台不保证数据的完整性、准确性和实时性。</p>

          <h2 className="text-lg font-semibold text-white">5. 知识产权</h2>
          <p>5.1 本平台的软件、界面设计、图标、文案等内容的知识产权归本平台所有。</p>
          <p>5.2 您创建的自定义策略、配置等内容的知识产权归您所有。</p>
          <p>5.3 未经本平台书面许可，您不得复制、修改、传播本平台的任何受保护内容。</p>

          <h2 className="text-lg font-semibold text-white">6. 服务变更与中断</h2>
          <p>6.1 我们可能因系统维护、升级或不可抗力等原因暂时中断服务，将尽量提前通知。</p>
          <p>6.2 我们保留修改、暂停或终止部分或全部服务的权利。对于免费服务的终止，我们不承担赔偿责任。</p>

          <h2 className="text-lg font-semibold text-white">7. 协议的变更</h2>
          <p>我们有权根据需要修订本协议。修订后的协议将在本平台公布，重大变更将通过站内通知告知您。如您在变更后继续使用本平台，则视为同意修订后的协议。</p>

          <h2 className="text-lg font-semibold text-white">8. 适用法律与争议解决</h2>
          <p>本协议的解释和执行适用中华人民共和国法律。因本协议引起的争议，双方应协商解决；协商不成的，任何一方可向本平台运营方所在地有管辖权的人民法院提起诉讼。</p>

          <h2 className="text-lg font-semibold text-white">9. 联系方式</h2>
          <p>如您对本协议有任何疑问，请联系：</p>
          <p>邮箱：<a href="mailto:easystudio@outlook.com" className="text-primary hover:underline">easystudio@outlook.com</a></p>
        </div>

        <div className="pt-6 border-t border-white/10 flex gap-4 text-sm text-white/40">
          <Link href="/privacy" className="hover:text-white/70 transition">隐私政策</Link>
          <Link href="/disclaimer" className="hover:text-white/70 transition">免责声明</Link>
        </div>
      </div>
    </>
  )
}
