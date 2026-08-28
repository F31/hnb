module github.com/F31/hnb/pkg/router

go 1.24.0

require (
	github.com/F31/hnb/pkg/tunnel v0.0.0-00010101000000-000000000000
	github.com/prometheus/client_golang v1.19.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace (
	github.com/F31/hnb/pkg/core => ../core
	github.com/F31/hnb/pkg/tunnel => ../tunnel
)
