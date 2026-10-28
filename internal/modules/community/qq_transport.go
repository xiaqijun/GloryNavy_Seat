package community

import (
	"glorynavy.local/seat/internal/httpapi"
	"net/http"
)

// Keep old URLs explicit without leaving a second live transport or writer.
func retiredOneBot(w http.ResponseWriter, r *http.Request) {
	httpapi.Failure(w, r, http.StatusGone, "qq_transport_retired", "已切换为官方 QQ 机器人，请刷新页面")
}
