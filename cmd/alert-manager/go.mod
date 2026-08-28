module github.com/F31/hnb/cmd/alert-manager

go 1.24.0

require (
	github.com/F31/hnb/pkg/alert v0.0.0-00010101000000-000000000000
	github.com/F31/hnb/pkg/messaging v0.0.0-00010101000000-000000000000
	github.com/lib/pq v1.10.9
	github.com/nats-io/nats.go v1.37.0
	github.com/prometheus/client_golang v1.19.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace (
	github.com/F31/hnb/pkg/alert => ../../pkg/alert
	github.com/F31/hnb/pkg/messaging => ../../pkg/messaging
)
