package engine

type CompensateType string

const (
	CompRollback    CompensateType = "rollback"
	CompDelete      CompensateType = "delete"
	CompRetainMark  CompensateType = "retain_mark"
	CompRetainNofiy CompensateType = "retain_notify"
	CompSkip        CompensateType = "skip"
)

type CompensationStrategy struct {
	ResourceType     string         `json:"resource_type"`
	Compensation     CompensateType `json:"compensation"`
	RetainData       bool           `json:"retain_data"`
	RequiresApproval bool           `json:"requires_approval"`
}

var defaultStrategies = map[string]CompensationStrategy{
	"database":   {ResourceType: "database", Compensation: CompRetainMark, RetainData: true, RequiresApproval: true},
	"volume":     {ResourceType: "volume", Compensation: CompRetainMark, RetainData: true, RequiresApproval: true},
	"backup":     {ResourceType: "backup", Compensation: CompRetainMark, RetainData: true, RequiresApproval: false},
	"deployment": {ResourceType: "deployment", Compensation: CompRollback, RetainData: false, RequiresApproval: false},
	"configmap":  {ResourceType: "configmap", Compensation: CompRollback, RetainData: false, RequiresApproval: false},
	"service":    {ResourceType: "service", Compensation: CompRollback, RetainData: false, RequiresApproval: false},
	"secret":     {ResourceType: "secret", Compensation: CompRollback, RetainData: false, RequiresApproval: false},
	"default":    {ResourceType: "default", Compensation: CompDelete, RetainData: false, RequiresApproval: false},
}

type CompensationEngine struct {
	strategies map[string]CompensationStrategy
}

func NewCompensationEngine() *CompensationEngine {
	strategies := make(map[string]CompensationStrategy, len(defaultStrategies))
	for k, v := range defaultStrategies {
		strategies[k] = v
	}
	return &CompensationEngine{strategies: strategies}
}

func (ce *CompensationEngine) GetStrategy(resourceType string) CompensationStrategy {
	if s, ok := ce.strategies[resourceType]; ok {
		return s
	}
	return ce.strategies["default"]
}

func (ce *CompensationEngine) RegisterStrategy(resourceType string, strategy CompensationStrategy) {
	ce.strategies[resourceType] = strategy
}

type CompensationRecord struct {
	ID               string         `json:"id"`
	OperationID      string         `json:"operation_id"`
	StepID           string         `json:"step_id"`
	ResourceType     string         `json:"resource_type"`
	ResourceID       string         `json:"resource_id"`
	CompensationType CompensateType `json:"compensation_type"`
	Status           string         `json:"status"`
	Data             map[string]any `json:"data"`
	Result           map[string]any `json:"result"`
	ErrorMessage     string         `json:"error_message"`
}
