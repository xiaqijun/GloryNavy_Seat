package feedback

import "net/http"

func (h Handler) rewardSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.Service.RewardSettings(r.Context(), h.Actor(r).ID)
	respond(w, r, v, err)
}
func (h Handler) configureReward(w http.ResponseWriter, r *http.Request) {
	var in RewardConfig
	if !read(w, r, &in) {
		return
	}
	v, err := h.Service.ConfigureReward(r.Context(), h.Actor(r).ID, in)
	respond(w, r, v, err)
}
