import { appConfig } from '@/lib/config'

const { appName } = appConfig()

export const META_DATA = {
  name: appName,
  title: `${appName} — Admin`,
  description: `${appName} administration console`,
  author: appName,
  username: 'Admin',
  keywords: `${appName}, admin, SaaS, entitlements, subscriptions`,
  version: '5.0.0',
  buyUrl: '__buyUrl__',
}

export const currentYear = new Date().getFullYear()
