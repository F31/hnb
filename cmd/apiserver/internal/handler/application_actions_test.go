package handler

import "testing"

func TestApplicationProviderStepType(t *testing.T) {
	cases := map[string]string{"deploy": "deploy", "upgrade": "workload.upgrade", "rollback": "workload.rollback", "delete": "delete"}
	for action, want := range cases {
		if got := applicationProviderStepType(action); got != want {
			t.Fatalf("%s -> %s, want %s", action, got, want)
		}
	}
}

func TestApplicationActionInputs(t *testing.T) {
	inputs, err := applicationActionInputs(applicationActionRequest{
		Action: "deploy", Image: "nginx@sha256:abc", ExpectedUID: "uid",
	}, "hnb-app", "cli-app", "demo-data", "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if inputs["namespace"] != "hnb-app" || inputs["name"] != "cli-app" || inputs["image"] != "nginx@sha256:abc" {
		t.Fatalf("inputs = %v", inputs)
	}
	if inputs["secret_reference"] != "demo-secret" || inputs["mount_reference"] != "/etc/hnb-secret" {
		t.Fatalf("secret aliases = %v", inputs)
	}

	if _, err := applicationActionInputs(applicationActionRequest{Action: "deploy"}, "n", "w", "", ""); err == nil {
		t.Fatal("deploy without image accepted")
	}
	if _, err := applicationActionInputs(applicationActionRequest{Action: "upgrade", SourceImage: "a", TargetImage: "b"}, "n", "w", "", ""); err == nil {
		t.Fatal("upgrade without expected UID accepted")
	}
	no := false
	if _, err := applicationActionInputs(applicationActionRequest{Action: "rollback", SourceImage: "a", TargetImage: "b", ExpectedUID: "u", DataIrreversible: &no}, "n", "w", "", "u"); err != nil {
		t.Fatalf("reversible rollback rejected: %v", err)
	}
}
