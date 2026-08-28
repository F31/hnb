package store

import (
	"database/sql"
	"encoding/json"

	"github.com/lib/pq"

	"github.com/F31/hnb/cmd/operation-worker/internal/engine"
)

type PlanStore struct {
	db *sql.DB
}

func NewPlanStore(db *sql.DB) *PlanStore {
	return &PlanStore{db: db}
}

func (s *PlanStore) SavePlan(plan *engine.ExecutionPlan) error {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	policyJSON, err := json.Marshal(plan.PolicyResult)
	if err != nil {
		return err
	}
	affinity := pq.Array(plan.NodeGroupAffinity)
	_, err = s.db.Exec(`
		INSERT INTO execution_plans (
			id, release_id, tenant_id, project_id, environment_id,
			plan_digest, plan_json, policy_result, node_group_affinity, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active')
		ON CONFLICT (id) DO NOTHING`,
		plan.ID, plan.ReleaseID, plan.TenantID, plan.ProjectID, plan.EnvironmentID,
		plan.PlanDigest, string(planJSON), string(policyJSON), affinity,
	)
	return err
}

func (s *PlanStore) GetPlan(id string) (*engine.ExecutionPlan, error) {
	var planJSON, policyJSON []byte
	var plan engine.ExecutionPlan
	var affinity []string

	err := s.db.QueryRow(`
		SELECT id, release_id, tenant_id, project_id, environment_id,
			plan_digest, plan_json, policy_result, node_group_affinity, created_at
		FROM execution_plans WHERE id = $1`, id).Scan(
		&plan.ID, &plan.ReleaseID, &plan.TenantID, &plan.ProjectID, &plan.EnvironmentID,
		&plan.PlanDigest, &planJSON, &policyJSON, pq.Array(&affinity), &plan.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return nil, err
	}
	if policyJSON != nil {
		if err := json.Unmarshal(policyJSON, &plan.PolicyResult); err != nil {
			return nil, err
		}
	}
	plan.NodeGroupAffinity = affinity

	return &plan, nil
}

func (s *PlanStore) GetPlanByDigest(digest string) (*engine.ExecutionPlan, error) {
	var planJSON, policyJSON []byte
	var plan engine.ExecutionPlan
	var affinity []string

	err := s.db.QueryRow(`
		SELECT id, release_id, tenant_id, project_id, environment_id,
			plan_digest, plan_json, policy_result, node_group_affinity, created_at
		FROM execution_plans WHERE plan_digest = $1`, digest).Scan(
		&plan.ID, &plan.ReleaseID, &plan.TenantID, &plan.ProjectID, &plan.EnvironmentID,
		&plan.PlanDigest, &planJSON, &policyJSON, pq.Array(&affinity), &plan.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return nil, err
	}
	if policyJSON != nil {
		if err := json.Unmarshal(policyJSON, &plan.PolicyResult); err != nil {
			return nil, err
		}
	}
	plan.NodeGroupAffinity = affinity

	return &plan, nil
}
