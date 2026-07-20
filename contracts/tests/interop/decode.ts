import assert from "node:assert/strict";
import { create, fromBinary, toBinary } from "@bufbuild/protobuf";

import {
  EventEnvelopeSchema,
} from "../../generated/typescript/proto/hnb/contracts/v1/contracts_pb.ts";

const input = Buffer.from(process.argv[2], "base64");
const envelope = fromBinary(EventEnvelopeSchema, input);

assert.equal(envelope.messageId, "018f6c2a-4a64-7b58-9cc3-9f70462f36c1");
assert.equal(envelope.correlationId, "018f6c2a-4a64-7b58-9cc3-9f70462f36c2");
assert.equal(envelope.idempotencyKey, "contract-echo-001");
assert.equal(envelope.occurredAt?.seconds, 1784548800n);
assert.equal(envelope.payload.case, "contractEchoed");
assert.equal(envelope.payload.value.context?.tenantId, "tenant-a");

const roundTrip = toBinary(EventEnvelopeSchema, create(EventEnvelopeSchema, envelope), {
  writeUnknownFields: true,
});
assert.ok(roundTrip.length >= input.length, "unknown fields must survive round trip");
