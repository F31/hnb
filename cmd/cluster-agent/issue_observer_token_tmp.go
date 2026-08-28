//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

func main() {
	keySet, err := iam.LoadPEMKeySet(os.Getenv("OBS_KEY_ID"), os.Getenv("OBS_PRIVATE_KEY"), map[string]string{os.Getenv("OBS_KEY_ID"): os.Getenv("OBS_PUBLIC_KEY")})
	if err != nil {
		fmt.Println("keyset:", err)
		os.Exit(1)
	}
	signer, err := iam.NewObserverTokenSigner(iam.ObserverTokenConfig{
		Issuer: os.Getenv("OBS_ISSUER"), Audience: os.Getenv("OBS_AUDIENCE"), TTL: 9 * time.Minute, Now: time.Now,
	}, keySet)
	if err != nil {
		fmt.Println("signer:", err)
		os.Exit(1)
	}
	token, err := signer.Sign(context.Background(), os.Getenv("OBS_SUBJECT"), iam.ObserverIdentity{
		TenantID: os.Getenv("OBS_TENANT"), TargetID: os.Getenv("OBS_TARGET"), TargetKind: "KubernetesTarget",
		ObserverID: os.Getenv("OBS_OBSERVER_ID"), ObserverKind: "Agent",
	}, "lease-"+os.Getenv("OBS_TARGET"), 1)
	if err != nil {
		fmt.Println("sign:", err)
		os.Exit(1)
	}
	fmt.Print(token)
}
