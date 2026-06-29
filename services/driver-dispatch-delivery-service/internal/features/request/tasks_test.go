package request

import (
	"encoding/json"
	"testing"
)

func TestRequestTaskPayloads(t *testing.T) {
	expire, err := NewExpireWindowTask(ExpireWindowPayload{BroadcastID: "b", BookingID: "k", AttemptNumber: 2})
	if err != nil {
		t.Fatal(err)
	}
	if expire.Type() != TaskExpireWindow {
		t.Fatalf("type=%s", expire.Type())
	}
	var expirePayload ExpireWindowPayload
	if err := json.Unmarshal(expire.Payload(), &expirePayload); err != nil || expirePayload.AttemptNumber != 2 {
		t.Fatalf("expire payload=%+v err=%v", expirePayload, err)
	}

	rebroadcast, err := NewReBroadcastTask(ReBroadcastPayload{BroadcastID: "b", BookingID: "k", AttemptNumber: 3, NewRadiusKM: 11})
	if err != nil {
		t.Fatal(err)
	}
	if rebroadcast.Type() != TaskReBroadcast {
		t.Fatalf("type=%s", rebroadcast.Type())
	}

	push, err := NewSendPushTask(SendPushPayload{ProviderID: "p", InboxID: "i", FareAmount: 150000})
	if err != nil {
		t.Fatal(err)
	}
	if push.Type() != TaskSendPush {
		t.Fatalf("type=%s", push.Type())
	}
	var pushPayload SendPushPayload
	if err := json.Unmarshal(push.Payload(), &pushPayload); err != nil || pushPayload.FareAmount != 150000 {
		t.Fatalf("push payload=%+v err=%v", pushPayload, err)
	}
}
