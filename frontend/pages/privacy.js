import Head from 'next/head'
import Link from 'next/link'

export default function PrivacyPolicy() {
  return (
    <>
      <Head><title>隐私政策 — 卧龙AI量化交易台</title></Head>
      <div className="max-w-3xl mx-auto pb-16 space-y-8">
        <header className="space-y-2">
          <h1 className="text-3xl font-bold tracking-tight">隐私政策</h1>
          <p className="text-sm text-white/45">最后更新日期：2026 年 4 月 2 日 · 生效日期：2026 年 4 月 2 日</p>
        </header>

        <div className="prose prose-invert prose-sm max-w-none space-y-6 text-white/75 leading-7">
          <p>欢迎使用「卧龙AI量化交易台」（以下简称「本平台」或「我们」）。我们深知个人信息对您的重要性，并会尽全力保护您的隐私安全。请您在使用本平台前仔细阅读本隐私政策。</p>
          <p>本平台由 Easy Studio Inc. 运营。</p>

          <h2 className="text-lg font-semibold text-white">1. 我们收集的信息</h2>

          <h3 className="text-base font-medium text-white/90">1.1 您主动提供的信息</h3>
          <ul className="list-disc pl-5 space-y-1">
            <li><strong>账户信息</strong>：注册邮箱、密码（加密存储），用于身份验证与登录。</li>
            <li><strong>投资画像</strong>：账户资金规模、风险偏好、投资目标、投资周期、投资经验、最大可承受回撤，用于个性化分析建议。</li>
            <li><strong>持仓记录</strong>：股票代码、持仓数量、买入均价、买入日期、备注，用于盈亏计算与展示。</li>
            <li><strong>关注池</strong>：您添加的股票代码及自定义名称，用于行情看板数据展示。</li>
            <li><strong>策略配置</strong>：自建策略的名称、参数、说明，用于信号评估与回测。</li>
            <li><strong>Webhook 配置</strong>：推送地址 URL、签名密钥，用于交易信号投递。</li>
          </ul>

          <h3 className="text-base font-medium text-white/90">1.2 自动收集的信息</h3>
          <ul className="list-disc pl-5 space-y-1">
            <li><strong>访问日志</strong>：IP 地址、浏览器类型、访问时间，用于安全防护与故障排查。</li>
            <li><strong>操作记录</strong>：登录/登出时间、信号触发记录、Webhook 投递记录，用于审计与问题追溯。</li>
          </ul>

          <h3 className="text-base font-medium text-white/90">1.3 我们不收集的信息</h3>
          <ul className="list-disc pl-5 space-y-1">
            <li>我们<strong>不收集</strong>您的真实姓名、身份证号、手机号、银行账户、证券账户等敏感个人信息。</li>
            <li>我们<strong>不访问</strong>您的证券交易账户，本平台不具备任何交易下单能力。</li>
            <li>我们<strong>不使用</strong>任何第三方行为追踪或广告 SDK。</li>
          </ul>

          <h2 className="text-lg font-semibold text-white">2. 信息的使用</h2>
          <p>我们仅在以下场景使用您的信息：</p>
          <ol className="list-decimal pl-5 space-y-1">
            <li><strong>提供核心服务</strong>：行情展示、策略回测、信号评估、Webhook 推送等功能的正常运行。</li>
            <li><strong>安全保障</strong>：识别异常登录、防止恶意攻击。</li>
            <li><strong>产品改进</strong>：通过匿名化的聚合统计数据（如用户总数、策略总数）了解产品使用情况。</li>
          </ol>
          <p>我们<strong>不会</strong>将您的个人信息用于广告推送或出售给第三方。</p>

          <h2 className="text-lg font-semibold text-white">3. 信息的存储与安全</h2>
          <ul className="list-disc pl-5 space-y-1">
            <li><strong>存储位置</strong>：您的数据存储在我们的云服务器上，数据库采用加密连接。</li>
            <li><strong>密码安全</strong>：账户密码使用 bcrypt 算法单向加密存储，我们无法获取您的明文密码。</li>
            <li><strong>访问控制</strong>：仅经授权的管理员可通过独立的管理后台查看聚合统计数据，不可查看单个用户的持仓、策略等隐私数据。</li>
            <li><strong>数据保留</strong>：账户注销后，我们将在 30 日内删除您的所有个人数据。</li>
          </ul>

          <h2 className="text-lg font-semibold text-white">4. 信息的共享</h2>
          <p>我们<strong>不会</strong>主动向任何第三方共享您的个人信息，但以下情况除外：</p>
          <ol className="list-decimal pl-5 space-y-1">
            <li><strong>经您明确同意</strong>：如您主动配置 Webhook 将信号推送至第三方服务。</li>
            <li><strong>法律法规要求</strong>：根据法律法规、司法程序或政府主管部门的强制要求。</li>
            <li><strong>安全事件</strong>：为保护用户或公众的人身安全、财产安全所必需的情况。</li>
          </ol>

          <h2 className="text-lg font-semibold text-white">5. 您的权利</h2>
          <p>您可以随时：</p>
          <ul className="list-disc pl-5 space-y-1">
            <li><strong>查看和修改</strong>个人信息（投资画像、持仓、关注池、策略等）。</li>
            <li><strong>删除</strong>您的策略、持仓记录、关注池等数据。</li>
            <li><strong>注销账户</strong>：联系我们删除您的全部数据。</li>
            <li><strong>导出数据</strong>：如需导出个人数据，请联系我们。</li>
          </ul>

          <h2 className="text-lg font-semibold text-white">6. 未成年人保护</h2>
          <p>本平台面向具有完全民事行为能力的成年人。如果我们发现未成年人未经监护人同意注册使用本平台，将主动删除相关信息。</p>

          <h2 className="text-lg font-semibold text-white">7. 政策更新</h2>
          <p>我们可能不时更新本隐私政策。更新后的政策将在本平台公布，重大变更将通过站内通知告知您。继续使用本平台即表示您同意更新后的隐私政策。</p>

          <h2 className="text-lg font-semibold text-white">8. 联系我们</h2>
          <p>如您对本隐私政策有任何疑问，请通过以下方式联系我们：</p>
          <p>邮箱：<a href="mailto:easystudio@outlook.com" className="text-primary hover:underline">easystudio@outlook.com</a></p>
        </div>

        <div className="pt-6 border-t border-white/10 flex gap-4 text-sm text-white/40">
          <Link href="/terms" className="hover:text-white/70 transition">用户协议</Link>
          <Link href="/disclaimer" className="hover:text-white/70 transition">免责声明</Link>
        </div>
      </div>
    </>
  )
}
