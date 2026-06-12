import Head from 'next/head'

export default function ComingSoonPage({ title }) {
  return (
    <>
      <Head>
        <title>{`${title} - 卧龙AI量化交易台`}</title>
      </Head>
      <div className="mx-auto flex min-h-[60vh] max-w-3xl items-center justify-center px-4 py-16">
        <section className="w-full rounded-2xl border border-border bg-card px-6 py-16 text-center">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground">{title}</h1>
          <p className="mt-4 text-base text-foreground-muted">敬请期待</p>
        </section>
      </div>
    </>
  )
}
