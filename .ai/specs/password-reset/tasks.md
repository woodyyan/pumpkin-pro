# Tasks

## Backend
- 新增密码重置 token 与申请记录表。
- 新增 forgot/reset/inspect API。
- 新增 mail provider 抽象与腾讯云实现。
- 重置密码后递增 credential version 并撤销所有 session。

## Frontend
- 新增 `/forgot-password` 页面。
- 新增 `/reset-password` 页面。
- 登录弹窗补“忘记密码”入口。
- 会话清理增加跨标签页广播。

## Test
- 补充 auth service / repository / config / handler 单测。
- 补充前端页面语法与登录态广播相关测试。
