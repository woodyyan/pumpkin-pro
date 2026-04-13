import React from 'react'

const SITE_URL = 'https://wolongtrader.top'

/** @type {import('next-sitemap').IConfig} */
module.exports = {
  siteUrl: SITE_URL,
  generateRobotsTxt: false, // 我们手动维护 public/robots.txt
  outDir: './public',
  generateIndexSitemap: false,
}
