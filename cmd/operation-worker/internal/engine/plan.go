package engine

type ExecutionPlan struct {
	ID                string            `json:"id"`
	ReleaseID         string            `json:"release_id"`
	TenantID          string            `json:"tenant_id"`
	ProjectID         string            `json:"project_id"`
	EnvironmentID     string            `json:"environment_id"`
	PlanDigest        string            `json:"plan_digest"`
	Steps             []StepSpec        `json:"steps"`
	Outputs           map[string]string `json:"outputs"`
	PolicyResult      *PolicyResult     `json:"policy_result"`
	NodeGroupAffinity []string          `json:"node_group_affinity,omitempty"`
	CreatedAt         string            `json:"created_at"`
}

type StepSpec struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	StepType   string            `json:"step_type"`
	DependsOn  []string          `json:"depends_on"`
	Optional   bool              `json:"optional"`
	Inputs     map[string]string `json:"inputs"`
	Outputs    []OutputBinding   `json:"outputs"`
	ProviderID string            `json:"provider_id"`
	Retry      RetryPolicy       `json:"retry"`
	TimeoutS   int               `json:"timeout_seconds"`
	Conditions map[string]string `json:"conditions"`
}

type OutputBinding struct {
	Name       string `json:"name"`
	FromStep   string `json:"from_step"`
	Expression string `json:"expression"`
}

type RetryPolicy struct {
	MaxRetries  int `json:"max_retries"`
	BaseDelayMs int `json:"base_delay_ms"`
	MaxDelayMs  int `json:"max_delay_ms"`
}

type PolicyResult struct {
	Passed    bool              `json:"passed"`
	Policies  []string          `json:"policies"`
	Decisions map[string]string `json:"decisions"`
}
