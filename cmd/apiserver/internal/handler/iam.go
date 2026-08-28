package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 20
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}
	return
}

type IAMHandler struct {
	auth         *iam.Authenticator
	tokenManager *iam.TokenManager
	rbac         *iam.RBACEngine
	store        *iam.IAMDBStore
}

func NewIAMHandler(auth *iam.Authenticator, tm *iam.TokenManager, rbac *iam.RBACEngine, store *iam.IAMDBStore) *IAMHandler {
	return &IAMHandler{
		auth:         auth,
		tokenManager: tm,
		rbac:         rbac,
		store:        store,
	}
}

func (h *IAMHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		MembershipID string `json:"membership_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	user, err := h.auth.Authenticate(req.Username, req.Password)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, 40100, "invalid credentials")
		return
	}
	access, refresh, err := h.tokenManager.Issue(r.Context(), user.ID, req.MembershipID)
	if err != nil {
		if errors.Is(err, iam.ErrNoAuthorizedTenant) || errors.Is(err, iam.ErrMembershipMismatch) {
			response.Error(w, http.StatusForbidden, 40301, "no authorized tenant membership")
			return
		}
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]any{
		"access_token":  access.Token,
		"refresh_token": refresh.Token,
		"expires_in":    int(time.Until(access.ExpiresAt).Seconds()),
		"subject_id":    access.UserID,
		"username":      user.Username,
	})
}

func (h *IAMHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	access, refresh, err := h.tokenManager.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, 40100, err.Error())
		return
	}
	response.Success(w, map[string]any{
		"access_token":  access.Token,
		"refresh_token": refresh.Token,
		"expires_in":    int(time.Until(access.ExpiresAt).Seconds()),
	})
}

func (h *IAMHandler) Logout(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]string{"status": "logged_out"})
}

func (h *IAMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	users, err := h.store.ListUsers()
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	type safeUser struct {
		ID          string    `json:"id"`
		Username    string    `json:"username"`
		Email       string    `json:"email,omitempty"`
		Phone       string    `json:"phone,omitempty"`
		DisplayName string    `json:"display_name,omitempty"`
		Source      string    `json:"source"`
		IsActive    bool      `json:"is_active"`
		CreatedAt   time.Time `json:"created_at"`
	}
	total := len(users)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	slice := users[start:end]
	result := make([]safeUser, len(slice))
	for i, u := range slice {
		result[i] = safeUser{
			ID: u.ID, Username: u.Username, Email: u.Email, Phone: u.Phone,
			DisplayName: u.DisplayName, Source: u.Source,
			IsActive: u.IsActive, CreatedAt: u.CreatedAt,
		}
	}
	response.Success(w, map[string]any{"items": result, "total": total})
}

func (h *IAMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Email       string `json:"email,omitempty"`
		Phone       string `json:"phone,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	user, err := h.auth.CreateUser(req.Username, req.Password, req.Email, req.Phone, req.DisplayName)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, map[string]any{"id": user.ID, "username": user.Username})
}

func (h *IAMHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	response.Success(w, h.rbac.ListRoles())
}

func (h *IAMHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	role, err := h.rbac.GetRole(id)
	if err != nil {
		response.NotFound(w, err.Error())
		return
	}
	response.Success(w, role)
}

func (h *IAMHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name,omitempty"`
		Scope       string   `json:"scope"`
		Verbs       []string `json:"verbs"`
		Resources   []string `json:"resources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Name == "" || req.Scope == "" {
		response.BadRequest(w, "name and scope are required")
		return
	}
	role := &iam.Role{
		ID:          uuid.NewString(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Scope:       iam.RoleScope(req.Scope),
		BuiltIn:     false,
		Rules:       []iam.PolicyRule{{Verbs: req.Verbs, Resources: req.Resources}},
	}
	if err := h.rbac.CreateRole(role); err != nil {
		response.Error(w, http.StatusConflict, 40900, err.Error())
		return
	}
	response.Created(w, role)
}

func (h *IAMHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.rbac.DeleteRole(id); err != nil {
		response.NotFound(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *IAMHandler) BindRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID  string `json:"user_id"`
		RoleID  string `json:"role_id"`
		Scope   string `json:"scope"`
		ScopeID string `json:"scope_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	binding := &iam.RoleBinding{
		ID: uuid.NewString(), RoleID: req.RoleID, UserID: req.UserID,
		Scope: iam.RoleScope(req.Scope), ScopeID: req.ScopeID,
	}
	if err := h.rbac.BindRole(binding); err != nil {
		response.Error(w, http.StatusConflict, 40900, err.Error())
		return
	}
	if err := h.store.SaveRoleBinding(binding); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Created(w, binding)
}

func (h *IAMHandler) UnbindRole(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	scope := r.PathValue("scope")
	scopeID := r.PathValue("scope_id")
	if err := h.rbac.UnbindRole(userID, iam.RoleScope(scope), scopeID); err != nil {
		response.NotFound(w, err.Error())
		return
	}
	if err := h.store.DeleteRoleBinding(userID, iam.RoleScope(scope), scopeID); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "unbound"})
}

func (h *IAMHandler) CheckPermission(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, 40100, "trusted context required")
		return
	}
	response.Success(w, map[string]bool{
		"allowed": h.rbac.HasPermission(trusted.SubjectID, r.URL.Query().Get("verb"),
			r.URL.Query().Get("resource"), iam.RoleScope(r.URL.Query().Get("scope")),
			r.URL.Query().Get("scope_id")),
	})
}

func (h *IAMHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, err := h.store.GetByID(id)
	if err != nil {
		response.NotFound(w, "user not found")
		return
	}
	response.Success(w, map[string]any{
		"id":           user.ID,
		"username":     user.Username,
		"email":        user.Email,
		"phone":        user.Phone,
		"display_name": user.DisplayName,
		"source":       user.Source,
		"is_active":    user.IsActive,
		"created_at":   user.CreatedAt,
		"updated_at":   user.UpdatedAt,
	})
}

func (h *IAMHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Email       *string `json:"email,omitempty"`
		Phone       *string `json:"phone,omitempty"`
		DisplayName *string `json:"display_name,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	user, err := h.store.GetByID(id)
	if err != nil {
		response.NotFound(w, "user not found")
		return
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if err := h.store.UpdateUser(user); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]any{
		"id": user.ID, "username": user.Username, "email": user.Email, "phone": user.Phone,
		"display_name": user.DisplayName, "is_active": user.IsActive,
	})
}

func (h *IAMHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteUser(id); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "deleted"})
}

func (h *IAMHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequest(w, "invalid body")
		return
	}
	if req.Password == "" {
		response.BadRequest(w, "password is required")
		return
	}
	user, err := h.store.GetByID(id)
	if err != nil {
		response.NotFound(w, "user not found")
		return
	}
	hash, err := h.auth.NewPasswordHash(req.Password)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	user.PasswordHash = hash
	if err := h.store.UpdatePasswordHash(id, hash); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, map[string]string{"status": "password_reset"})
}

func (h *IAMHandler) ListRoleBindings(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	page, pageSize := parsePagination(r)
	var bindings []iam.RoleBinding
	var err error
	if userID != "" {
		bindings, err = h.store.ListRoleBindingsByUser(userID)
	} else {
		bindings, err = h.store.ListRoleBindings()
	}
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	total := len(bindings)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	response.Success(w, map[string]any{"items": bindings[start:end], "total": total})
}
