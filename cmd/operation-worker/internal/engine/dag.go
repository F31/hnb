package engine

import (
	"fmt"
	"strings"
)

type DAGResolver struct {
	steps map[string]*StepSpec
	order []string
}

func NewDAGResolver(steps []StepSpec) *DAGResolver {
	s := make(map[string]*StepSpec, len(steps))
	for i := range steps {
		s[steps[i].ID] = &steps[i]
	}
	return &DAGResolver{steps: s}
}

func (r *DAGResolver) Resolve() ([]string, error) {
	inDegree := make(map[string]int, len(r.steps))
	deps := make(map[string][]string, len(r.steps))

	for id, step := range r.steps {
		inDegree[id] = 0
		for _, dep := range step.DependsOn {
			if _, ok := r.steps[dep]; !ok {
				return nil, fmt.Errorf("step %q depends on unknown step %q", id, dep)
			}
			deps[dep] = append(deps[dep], id)
		}
	}

	for _, step := range r.steps {
		inDegree[step.ID] += len(step.DependsOn)
	}

	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, dependent := range deps[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(r.steps) {
		return nil, fmt.Errorf("cycle detected: resolved %d of %d steps", len(order), len(r.steps))
	}

	r.order = order
	return order, nil
}

func (r *DAGResolver) ExecutionLevels() ([][]string, error) {
	_, err := r.Resolve()
	if err != nil {
		return nil, err
	}

	depth := make(map[string]int, len(r.steps))
	maxDepth := 0

	for _, id := range r.order {
		step := r.steps[id]
		d := 0
		for _, dep := range step.DependsOn {
			if depth[dep]+1 > d {
				d = depth[dep] + 1
			}
		}
		depth[id] = d
		if d > maxDepth {
			maxDepth = d
		}
	}

	levels := make([][]string, maxDepth+1)
	for id, d := range depth {
		levels[d] = append(levels[d], id)
	}

	return levels, nil
}

func (r *DAGResolver) ReadySteps(completed map[string]bool) []string {
	var ready []string
	for _, step := range r.steps {
		if completed[step.ID] {
			continue
		}
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if !completed[dep] {
				allDepsCompleted = false
				break
			}
		}
		if allDepsCompleted {
			ready = append(ready, step.ID)
		}
	}
	return ready
}

type OutputResolver struct {
	stepOutputs map[string]map[string]string
}

func NewOutputResolver() *OutputResolver {
	return &OutputResolver{stepOutputs: make(map[string]map[string]string)}
}

func (r *OutputResolver) SetStepOutput(stepID string, outputs map[string]string) {
	r.stepOutputs[stepID] = outputs
}

func (r *OutputResolver) ResolveBinding(binding OutputBinding) (string, error) {
	outputs, ok := r.stepOutputs[binding.FromStep]
	if !ok {
		return "", fmt.Errorf("no output from step %q", binding.FromStep)
	}
	expr := strings.TrimPrefix(binding.Expression, "$.")
	if val, ok := outputs[expr]; ok {
		return val, nil
	}
	return "", fmt.Errorf("output %q not found in step %q", expr, binding.FromStep)
}
