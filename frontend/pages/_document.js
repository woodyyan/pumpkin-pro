import { Html, Head, Main, NextScript } from 'next/document'

/**
 * Custom Document — injects a blocking script to prevent theme flash (FOUC).
 *
 * This script runs BEFORE React hydration and sets the correct `class` on <html>
 * so that CSS variables resolve to the right theme from the very first paint.
 */
export default function Document() {
  return (
    <Html lang="zh-CN">
      <Head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
(function(){
  try {
    var theme = localStorage.getItem('wolong_theme');
    if (theme === 'light') {
      document.documentElement.classList.add('light');
    } else if (theme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      // 'system' or unset — follow OS preference
      if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.add('light');
      }
    }

    // Apply smooth transition class AFTER initial paint to avoid FOUC
    window.addEventListener('load', function(){
      document.documentElement.classList.add('theme-transition');
    });
  } catch(e) {
    // localStorage unavailable — fallback to dark
    document.documentElement.classList.add('dark');
  }
})();
            `,
          }}
        />
      </Head>
      <body>
        <Main />
        <NextScript />
      </body>
    </Html>
  )
}
