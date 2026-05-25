# 密码找回功能需求

## 背景

当前系统只支持邮箱+密码登录，没有找回密码功能。用户忘记密码后无法恢复账号访问。

## 目标

为已注册用户提供通过邮箱验证的密码重置流程，确保安全性和可用性。

## 功能需求

1. 用户可在登录页发起「忘记密码」请求，输入注册邮箱后收到重置邮件
2. 邮件包含重置深链，点击后跳转到独立的 `/reset-password` 页面
3. 用户在 `/reset-password` 页面设置新密码
4. 重置成功后自动撤销该用户所有 session，强制重新登录
5. 登录页新增 `/forgot-password` 页面入口

## 非功能需求

1. 重置 token 有效期 30 分钟，过期作废
2. 同一邮箱 60 秒内不可重复申请（冷却时间）
3. 同一 IP 每小时最多 10 次请求
4. 同一邮箱每小时最多 3 次请求
5. 重置 token 只能消费一次，通过数据库原子更新保证
6. 更新密码 + 消费 token + 撤销 session 必须在同一事务中
7. 邮件模板不带用户敏感数据，只给深链和有效期说明
8. 本地开发使用 mock provider，不发送真实邮件

## 约束

1. 邮件发送使用腾讯云邮件推送 API（非 SMTP）
2. 发送域名：`wolongtrader.top`
3. 固定发件人地址：`no-reply@wolongtrader.top`
4. 前端公共域名：`https://wolongtrader.top`
5. 单实例部署，无多实例一致性问题
6. 本阶段不考虑用户已失去邮箱控制权的情况
7. 邮件模板需提供 HTML + 纯文本两个版本，HTML 仅支持 UTF-8，文件大小不超过 400KB

## 已知信息

- 后端技术栈：Go + GORM + SQLite
- 前端技术栈：Next.js Pages Router + Tailwind CSS
- 认证模块位于 `backend/store/auth/`
- 前端认证上下文位于 `frontend/lib/auth-context.js`
- 前端认证存储位于 `frontend/lib/auth-storage.js`
