# QQ 建筑提醒恢复纯文本（2026-11-05）

后端 `v0.1.0-qq-structure-plaintext-20261105` 来自干净且已推送的 main `084fe5571cfe7806e6039ce9b237042224259eee`；发布包 SHA256 `05bd4ea105ef264b36886b62a60408cd60fc8caeafe29b85768577e1dedf402b`，包含六个后端程序，服务器清单校验通过。前端未切换，Goose 保持 117。

建筑提醒恢复原纯文本发送，保留建筑名称、类型、星系和北京时间。隔离 PostgreSQL 与模拟 QQ 接口验证消息类型、正文、跨日时间、去重、旧告警失效及未知结果不重发；Go 单元测试、vet 和前端构建通过。此次未向群发送测试，真实群内显示待验收。GitNexus 索引落后，未识别新增的 Markdown 符号；已核对实际调用范围。

标准脚本备份 `/var/backups/glorynavy/before-v0.1.0-qq-structure-plaintext-20261105-20261105T011836Z.dump`，295561091 字节、权限 0600，本次未做完整恢复演练。发布前后建筑监测与机器人保持启用、35 条建筑基线及 8 条提醒记录不变、无待发送记录。服务 active/running、NRestarts=0、ready 200，QQ WebSocket 重连，公网首页 200。
