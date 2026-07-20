package main

import (
	"encoding/base64"
	"fmt"
	"time"

	contractsv1 "github.com/F31/hnb/contracts/generated/go/proto/hnb/contracts/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	message := &contractsv1.EventEnvelope{
		MessageId:      "018f6c2a-4a64-7b58-9cc3-9f70462f36c1",
		MessageType:    "hnb.event.contract.echoed.v1",
		SchemaVersion:  "1.0.0",
		OccurredAt:     timestamppb.New(time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)),
		TenantId:      "tenant-a",
		CorrelationId: "018f6c2a-4a64-7b58-9cc3-9f70462f36c2",
		IdempotencyKey: "contract-echo-001",
		Payload: &contractsv1.EventEnvelope_ContractEchoed{ContractEchoed: &contractsv1.ContractEchoed{
			Context: &contractsv1.RequestContext{
				TenantId:      "tenant-a",
				ActorId:       "user-42",
				CorrelationId: "018f6c2a-4a64-7b58-9cc3-9f70462f36c2",
			},
			Value: "contract fixture",
		}},
	}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		panic(err)
	}
	// Unknown field 99 verifies that a newer producer does not break an older consumer.
	encoded = append(encoded, 0x98, 0x06, 0x01)
	fmt.Print(base64.StdEncoding.EncodeToString(encoded))
}
