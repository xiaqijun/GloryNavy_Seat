# EVE Sentry 预警平台密钥与时间计费

本站负责密钥创建/更新、账号归属、按小时价格和管理员收费开关。密钥明文以服务端 `EVE_TOKEN_KEY` 派生密钥加密保存，仅在当前账号的受保护列表响应中返回；不写入浏览器存储、URL 或日志。页面只提供复制和更新按钮，不自动轮换。

迁移前已创建的旧密钥没有可恢复的明文，需手动更新一次完成加密保存；更新后可随时复制。

## API

- `GET/POST /api/v1/sentry/keys`：读取或创建当前账号密钥。
- `POST /api/v1/sentry/keys/{id}/rotate`：手动更新密钥，加密保存并返回完整内容；之后列表仍可读取并复制。
- `GET/PUT /api/v1/sentry/time-pricing`：管理员读取或保存预警价格、监控奖励价格（果壳币/小时）及收费开关。
- `GET /api/v1/sentry/alert-usage`、`GET /api/v1/sentry/alert-consumptions`：读取账号级余额、花费和奖励流水。
- `GET /api/v1/sentry/monitor-rewards`：读取最近奖励区间，并返回全部已入账奖励的 `total_minor` 和 `total_count`；列表只展示最近记录。

收费开关保存时同步调用 Sentry 的 `PUT /api/v1/integrations/seat/alert-consumption`。远端同步失败不会提交本地开关。

## 计费口径

新收费只读取 Sentry 的 `GET /api/v1/integrations/seat/client-usage`。每条记录是认证客户端相邻有效心跳之间的服务端在线区间，Seat 按区间秒数乘小时价格幂等结算。事件次数、事件投递、ACK、远端预付授权、释放和退款不参与收费，相关接口与历史兼容表已删除；Seat 内部仅保留结算所需的原子币账引用。

监控奖励读取 `GET /api/v1/integrations/seat/monitor-contributions`，每个星系只有主节点贡献有效在线时长。同一账号选择多个预警星系时，各星系独立计量并合并到账号级果壳币流水。

服务令牌只保存在两端服务端环境中，不进入浏览器或文档；生产入口仍为 `https://seat.kisectool.com`。
