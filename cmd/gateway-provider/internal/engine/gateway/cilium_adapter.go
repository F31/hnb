package gateway

const (
	CiliumGatewayClass = "cilium"
)

func NewCiliumAdapter(gatewayClassName string) *GenericAdapter {
	extraLabels := map[string]string{"hnb.cloud/adapter": "cilium"}
	extraAnnotations := map[string]string{"hnb.cloud/adapter": "cilium"}
	return NewGenericAdapter("cilium", gatewayClassName, extraLabels, extraAnnotations)
}