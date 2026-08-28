package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

// AgentOnboarding 是“纳管集群接入闭环”的服务端环节（对齐 KubeSphere 成员集群
// Agent 下发 / Rancher 集群注册命令）：浏览器无法直连目标集群，因此由 apiserver
// 在校验调用者对该目标有权之后，签发绑定 (tenant, cluster) 的 agent-tunnel
// 令牌，并渲染 kubectl 可直接 apply 的 cluster-agent 部署清单。管理员在目标集群
// 执行返回的安装命令后，Agent 经隧道回连平台，观测链路随之建立。
//
// 安全约束：
//   - 目标按 tenant_id 精确过滤（跨租户/共享目标一律 404，不泄露存在性）；
//   - 仅 KubernetesTarget 支持 Agent 接入（边缘运行时经 CloudCore，不适用）；
//   - 令牌为 hnb.agent-tunnel/v1 专用 profile，校验端强制 (tenant, cluster)
//     绑定一致；签发时点即授权时点，令牌本身不再携带用户身份；
//   - 清单中不出现平台侧密钥；Agent 在目标集群内使用自身 ServiceAccount 观测。
type AgentOnboardingHandler struct {
	db            *sql.DB
	signer        *iam.AgentTunnelTokenSigner
	publicBaseURL string
	agentImage    string
}

func NewAgentOnboardingHandler(db *sql.DB, signer *iam.AgentTunnelTokenSigner, publicBaseURL, agentImage string) *AgentOnboardingHandler {
	return &AgentOnboardingHandler{
		db:            db,
		signer:        signer,
		publicBaseURL: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		agentImage:    strings.TrimSpace(agentImage),
	}
}

type agentOnboardingPayload struct {
	ClusterID      string `json:"clusterId"`
	TenantID       string `json:"tenantId"`
	DisplayName    string `json:"displayName"`
	TunnelURL      string `json:"tunnelUrl"`
	Token          string `json:"token"`
	TokenExpiry    string `json:"tokenExpiry"`
	Namespace      string `json:"namespace"`
	Manifest       string `json:"manifest"`
	InstallCommand string `json:"installCommand"`
}

func (h *AgentOnboardingHandler) AgentOnboarding(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	if h.db == nil || h.signer == nil {
		response.ServiceUnavailable(w, "agent onboarding is not configured for this deployment")
		return
	}
	id := r.PathValue("id")

	var (
		name        string
		displayName string
		targetType  string
	)
	err := h.db.QueryRowContext(r.Context(), `
		SELECT rt.name, COALESCE(rt.display_name, rt.name), rt.target_type
		FROM runtime_targets rt
		WHERE rt.id = $1 AND rt.tenant_id = $2`, id, trusted.TenantID).
		Scan(&name, &displayName, &targetType)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "cluster not found")
		return
	}
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	if targetType != "kubernetes" {
		response.Forbidden(w, "agent onboarding is only available for Kubernetes clusters")
		return
	}
	if h.agentImage == "" {
		response.ServiceUnavailable(w, "agent image is not configured for this deployment")
		return
	}

	token, expiry, err := h.signer.Sign(r.Context(), trusted.TenantID, id)
	if err != nil {
		// 令牌签发失败属于服务端配置/密钥问题，不回传内部细节。
		response.InternalError(w, "agent onboarding token issuance failed")
		return
	}

	namespace := agentOnboardingNamespace(trusted.TenantID)
	tunnelURL := h.tunnelURL(r)
	manifest := renderAgentOnboardingManifest(namespace, trusted.TenantID, id, tunnelURL, token, h.agentImage)
	installCommand := "kubectl apply -f - <<'HNB_AGENT_EOF'\n" + manifest + "HNB_AGENT_EOF"

	// 与集群 BFF 其余端点一致：成功路径返回裸载荷（无 code/message/data 信封），
	// 前端 api-client 不做信封解包。
	writeJSONRaw(w, agentOnboardingPayload{
		ClusterID:      id,
		TenantID:       trusted.TenantID,
		DisplayName:    displayName,
		TunnelURL:      tunnelURL,
		Token:          token,
		TokenExpiry:    expiry.UTC().Format(time.RFC3339),
		Namespace:      namespace,
		Manifest:       manifest,
		InstallCommand: installCommand,
	})
}

// tunnelURL 计算目标集群可回连的隧道 WebSocket 地址：优先 PUBLIC_BASE_URL
// （部署侧显式声明的对外地址），否则按请求推导（X-Forwarded-Proto / TLS / Host）。
func (h *AgentOnboardingHandler) tunnelURL(r *http.Request) string {
	if h.publicBaseURL != "" {
		return upgradeScheme(h.publicBaseURL) + "/tunnel"
	}
	scheme := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + "/tunnel"
}

func upgradeScheme(baseURL string) string {
	if strings.HasPrefix(baseURL, "https://") {
		return "wss://" + strings.TrimPrefix(baseURL, "https://")
	}
	if strings.HasPrefix(baseURL, "http://") {
		return "ws://" + strings.TrimPrefix(baseURL, "http://")
	}
	return baseURL
}

// agentOnboardingNamespace 生成每租户独立的命名空间，避免同一目标集群被多个
// 租户纳管时资源互相覆盖。命名规则：hnb-agent-<sanitized-tenant>（K8s 名称
// 安全；非法字符折叠为 '-'；空结果退化为租户 ID 的短哈希）。
func agentOnboardingNamespace(tenantID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(tenantID) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		sum := sha256.Sum256([]byte(tenantID))
		name = hex.EncodeToString(sum[:4])
	}
	if len(name) > 40 {
		name = name[:40]
	}
	return "hnb-agent-" + name
}

// renderAgentOnboardingManifest 渲染 kubectl apply 的多文档清单：
// Namespace / ServiceAccount / ClusterRole(只读观测) / ClusterRoleBinding /
// Secret(agent-tunnel 令牌) / Deployment(hnb/cluster-agent)。
// Agent 在目标集群内以自身 ServiceAccount 访问 K8s API（in-cluster），
// 因此清单中不需要也不允许出现纳管凭据（kubeconfig）。
func renderAgentOnboardingManifest(namespace, tenantID, clusterID, tunnelURL, token, agentImage string) string {
	roleName := namespace + "-observer"
	const inClusterCA = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	const inClusterToken = "/var/run/secrets/kubernetes.io/serviceaccount/token"

	var b strings.Builder
	b.WriteString("# HNB cluster-agent onboarding manifest (auto-generated, do not edit by hand)\n")
	b.WriteString("# cluster: " + clusterID + " / tenant: " + tenantID + "\n")
	b.WriteString("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: " + namespace + "\n")
	b.WriteString("---\napiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: hnb-cluster-agent\n  namespace: " + namespace + "\n")
	b.WriteString("---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata:\n  name: " + roleName + "\nrules:\n")
	b.WriteString("  - apiGroups: [\"\"]\n    resources: [\"nodes\", \"namespaces\", \"services\", \"persistentvolumes\", \"persistentvolumeclaims\", \"configmaps\", \"secrets\", \"events\", \"pods\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("  - apiGroups: [\"\"]\n    resources: [\"pods/log\"]\n    verbs: [\"get\"]\n")
	b.WriteString("  - apiGroups: [\"apps\"]\n    resources: [\"deployments\", \"statefulsets\", \"daemonsets\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("  - apiGroups: [\"batch\"]\n    resources: [\"jobs\", \"cronjobs\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("  - apiGroups: [\"networking.k8s.io\"]\n    resources: [\"ingresses\", \"networkpolicies\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("  - apiGroups: [\"storage.k8s.io\"]\n    resources: [\"storageclasses\", \"csidrivers\", \"csinodes\", \"csistoragecapacities\", \"volumeattachments\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("  - apiGroups: [\"snapshot.storage.k8s.io\"]\n    resources: [\"volumesnapshots\", \"volumesnapshotclasses\", \"volumesnapshotcontents\"]\n    verbs: [\"get\", \"list\", \"watch\"]\n")
	b.WriteString("---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRoleBinding\nmetadata:\n  name: " + roleName + "\nroleRef:\n  apiGroup: rbac.authorization.k8s.io\n  kind: ClusterRole\n  name: " + roleName + "\nsubjects:\n  - kind: ServiceAccount\n    name: hnb-cluster-agent\n    namespace: " + namespace + "\n")
	b.WriteString("---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: hnb-cluster-agent-token\n  namespace: " + namespace + "\ntype: Opaque\nstringData:\n  agent-token: " + quoteYAMLValue(token) + "\n")
	b.WriteString("---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: hnb-cluster-agent\n  namespace: " + namespace + "\n  labels:\n    app.kubernetes.io/name: hnb-cluster-agent\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n      app.kubernetes.io/name: hnb-cluster-agent\n  template:\n    metadata:\n      labels:\n        app.kubernetes.io/name: hnb-cluster-agent\n    spec:\n      serviceAccountName: hnb-cluster-agent\n      containers:\n        - name: agent\n          image: " + quoteYAMLValue(agentImage) + "\n          env:\n")
	b.WriteString("            - name: TUNNEL_URL\n              value: " + quoteYAMLValue(tunnelURL) + "\n")
	b.WriteString("            - name: AGENT_TOKEN_FILE\n              value: /etc/hnb/agent-token/agent-token\n")
	b.WriteString("            - name: TENANT_ID\n              value: " + quoteYAMLValue(tenantID) + "\n")
	b.WriteString("            - name: CLUSTER_ID\n              value: " + quoteYAMLValue(clusterID) + "\n")
	b.WriteString("            - name: KUBE_API\n              value: https://kubernetes.default.svc\n")
	b.WriteString("            - name: KUBE_TOKEN_FILE\n              value: " + inClusterToken + "\n")
	b.WriteString("            - name: KUBE_CA_FILE\n              value: " + inClusterCA + "\n")
	b.WriteString("            - name: RECONNECT_INTERVAL\n              value: \"10\"\n")
	b.WriteString("            - name: OBSERVATION_INTERVAL\n              value: \"60s\"\n")
	b.WriteString("          volumeMounts:\n            - name: agent-token\n              mountPath: /etc/hnb/agent-token\n              readOnly: true\n")
	b.WriteString("          resources:\n            requests:\n              cpu: 100m\n              memory: 128Mi\n            limits:\n              cpu: 500m\n              memory: 512Mi\n")
	b.WriteString("      volumes:\n        - name: agent-token\n          secret:\n            secretName: hnb-cluster-agent-token\n")
	return b.String()
}

// quoteYAMLValue 将值渲染为 YAML 双引号标量（JSON 字符串即合法的 YAML 双引号
// 标量），避免令牌/URL 中的特殊字符破坏清单结构。
func quoteYAMLValue(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
