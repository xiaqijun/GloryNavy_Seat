# 意见反馈

登录后从导航“意见反馈”（`/feedback`）提交 BUG 或功能需求，填写标题和详细描述。用户可以查看自己的反馈、处理状态和回复，并补充说明。当前站点管理员切换“全部反馈”处理所有成员的反馈，可以回复、修改状态和重新打开；军团管理员、专员或 `access.manage` 不代替站点管理员身份。

状态为“待处理、处理中、已解决、已关闭”。提交内容与历史回复不可修改或删除，处理历史保存当时作者名称、管理员身份与时间。用户内容按纯文本显示。支持类型/状态筛选、每页 30 条、稳定 ID 倒序分页，以及 `?ticket=<id>` 的受保护详情链接。截图保持私有，没有公开反馈广场；反馈进度和待处理提醒按独立提醒模块配置。

## 截图（Goose 99/100）

提交与回复使用 `multipart/form-data`：一个 `payload` 字段保存严格 JSON 业务参数（不含 images，最多 32 KiB），重复 `image` 文件字段携带 0–3 张 PNG/JPG。取消文件体积、整次 multipart 请求和像素数的固定上限。保留旧 JSON/base64 协议供旧客户端使用，其 8 MiB 请求保护不变；新版页面有图片时使用 multipart，不受该兼容限制。正文规则、类型、权限和每次 3 张保持不变。

浏览器使用 File 与 Blob URL 预览，不把原图转成大段 base64；支持选择、粘贴、移除和失败保稿。服务端在鉴权后流式写私有临时文件，检查签名/尺寸信息、PNG 分块 CRC 与结束标记、JPEG 标记流与结束标记，不分配整张图的像素缓冲、不进行完整像素解压。正常成功或失败均关闭并删除临时文件；生产 systemd PrivateTmp 保持启用。只对截图 multipart/原图读取解除普通 API 的总读写 deadline，边缘保留 300 秒无数据传输超时；未取消其他接口保护。

Goose 100 在 `feedback_images` 保存字节数并兼容旧 content，新增私有 `feedback_image_chunks`，每块最多 1 MiB，按图片/序号存储。新图片元数据、所有分块、正文/回复和 revision 在同一事务提交，失败全部回滚；SHA-256 比较仍覆盖原图全部字节和顺序。新旧图片都按父反馈当前归属/当前站点管理员鉴权；账号合并无需复制分块或新增参与方。详情只返回 `images: [{id,mime}]`，列表/提醒不含原图。

`GET /api/v1/feedback/tickets/{id}/images/{image}` 逐块读取原图，返回正确 MIME/Content-Length、`private, no-store` 与 `nosniff`；无权、错父单或不存在均 404。前端缩略图链接到此接口。已有分块时禁止回退忽略分块的旧程序，Goose 100 Down 拒绝有分块的库。备份仍使用原数据库备份，含全部图片分块，无外部永久文件目录。

代理部署必须同步 `deploy/edge-api-proxy.conf` 中专用 `/api/v1/feedback/` 段：`client_max_body_size 0`、关闭请求/响应缓冲并放宽无数据超时。其他业务继续沿用原 8m 等限制。发布先后端、代理配置检查/重载、再前端；生产不提交测试反馈或发送通知。

## 接口与权限

- `GET /api/v1/feedback/tickets`：默认 `scope=mine`；`scope=all` 仅当前站点管理员；可选 `kind=bug|feature`、`status=submitted|in_progress|resolved|closed`、十进制 `before`。返回 `tickets`、`next_before` 和 `administrator`，列表不含正文。
- `POST /api/v1/feedback/tickets`：`request_key` 为非零 UUID，`kind`、`title`、`body` 必填；标题最多 120 字符、正文最多 6000 字符，去除首尾空白。返回已保存反馈。
- `GET /api/v1/feedback/tickets/{id}`：返回 `ticket`、`messages` 和 `administrator`。仅归属账号或当前站点管理员可读，其他账号与不存在对象均返回 404。
- `POST /api/v1/feedback/tickets/{id}/messages`：`request_key`、原 `revision`、`body`、`status`；正文最多 2000 字符，状态可空表示保持不变，仅管理员可提交非空状态。纯状态操作允许空正文。版本冲突返回 409，刷新后保留草稿再提交。

全部入口由会话与 `feedback.self` 保护，写操作要求同源 Origin 和 CSRF。该 self 能力不加入可委派管理权限目录。每次业务读取/处理重查当前站点管理员，不依赖页面标记。兼容 JSON 限制 8 MiB、multipart 元数据限制 32 KiB；拒绝未知字段和额外 JSON；协议枚举不翻译。错误包括 400、401、403、404、409、429、503。完整响应见 [OpenAPI](../../api/openapi.yaml)。

同一原提交者和请求键重试仅返回已保存单据；相同键不同内容返回 409。回复同样幂等，冲突检查在重放识别之后。每账号滚动 24 小时最多 20 次新反馈、100 次回复（管理员回复不受日限额）；单条反馈最多 100 条回复/状态历史，达到上限可新开反馈。账号写锁串行化日限额，反馈行锁及 revision 防止并发覆盖；失败事务不留下半条记录。

## 数据与部署

Goose 96 建立 `feedback_tickets`、`feedback_messages` 和索引，SQL/sqlc 归 `feedback/internal/store`。宿主仅注入当前管理员检查、会话作者和 `identity.LockActiveAccounts`。账号锁先于模块写锁/反馈行锁；数据库活跃账号触发器防止迟到请求写向已合并账号。没有 ESI 调用、角色令牌、新 scope 或后台任务。

合并参与方始终注册，即使模块停用。反馈归属迁往目标账号，首次 `original_account_id`、原提交者、回复作者与时间不变；revision 增加，原处理请求失效。预览摘要包含两账号反馈 ID/revision 指纹，任何回复或状态变化会使预览过期。不迁移来源管理员权限。

从已推送的干净 main 发布同版前后端；备份数据库与配置，执行最新 Goose/River 迁移，在原 `MODULES` 清单追加 `feedback` 并保留 `identity/access` 及其依赖。移除模块只隐藏入口，不删历史。已有反馈时 Down 被拒绝，也不能回退到遗漏反馈合并参与方的旧程序；需保留 schema 和合并逻辑。

当前发布和验证状态见[项目状态](../project-status.md)，布局见[页面说明](../ui/feedback.md)。
