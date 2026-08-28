package handler

import (
	"net/http"
	"strings"

	"github.com/F31/hnb/cmd/apiserver/internal/capability"
	"github.com/F31/hnb/cmd/apiserver/internal/response"
)

// CapabilityHandler serves the staged cluster capability gates to the Web
// Console (KERNEL-016). The console consults it to hide menus/schemas/actions
// for disabled stages; the server still fail-closes every gated route.
type CapabilityHandler struct{ caps capability.Set }

func NewCapabilityHandler(caps capability.Set) *CapabilityHandler {
	return &CapabilityHandler{caps: caps}
}

// List returns the enabled stage list.
func (h *CapabilityHandler) List(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]any{"stages": h.caps.EnabledStages()})
}

// Get reports availability for a single capability name (fail closed).
func (h *CapabilityHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		response.BadRequest(w, "capability name is required")
		return
	}
	response.Success(w, map[string]any{"available": h.caps.Has(name)})
}
