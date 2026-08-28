//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

const clusterID = "79eb7403-2e06-4502-901a-420e3c40cd55"

type tunnelIdentity struct{}

func (tunnelIdentity) ResolveUserIdentity(context.Context, string, string) (*iam.Identity, error) {
	return &iam.Identity{UserID: "cluster-agent", SubjectID: "cluster-agent-graphify", SubjectType: "service", MembershipID: "cluster-agent-graphify", TenantID: "tenant-dev"}, nil
}

func (tunnelIdentity) ResolveMembership(ctx context.Context, userID, membershipID string) (*iam.Identity, error) {
	return tunnelIdentity{}.ResolveUserIdentity(ctx, userID, membershipID)
}

type tunnelPermissions struct{}

func (tunnelPermissions) ResolvePermissions(context.Context, string, string, string) (string, []iam.ScopedPermission, error) {
	return "cluster-agent:1", []iam.ScopedPermission{{ResourceKind: "cluster", ResourceID: clusterID, Action: iam.ActionExecute, TenantID: "tenant-dev"}}, nil
}

type discardRefreshStore struct{}

func (discardRefreshStore) CreateRefreshToken(context.Context, iam.RefreshTokenRecord) error { return nil }

func (discardRefreshStore) RotateRefreshToken(context.Context, string, iam.RefreshTokenRecord, time.Time) (*iam.RefreshTokenRecord, error) {
	return nil, nil
}

func main() {
	keyID := os.Getenv("HNB_KEY_ID")
	keySet, err := iam.LoadPEMKeySet(keyID, os.Getenv("HNB_PRIVATE_KEY"), map[string]string{keyID: os.Getenv("HNB_PUBLIC_KEY")})
	if err != nil {
		panic(err)
	}
	manager, err := iam.NewTokenManager(iam.TokenManagerConfig{
		Issuer: os.Getenv("HNB_ISSUER"), Audience: "hnb-apiserver-tunnel", Audiences: []string{"hnb-apiserver-tunnel"},
		AccessTTL: iam.MaxAccessTokenTTL, RefreshTTL: time.Minute, Now: time.Now,
	}, keySet, keySet, tunnelIdentity{}, tunnelPermissions{}, discardRefreshStore{})
	if err != nil {
		panic(err)
	}
	access, _, err := manager.Issue(context.Background(), "cluster-agent", "cluster-agent-graphify")
	if err != nil {
		panic(err)
	}
	fmt.Print(access.Token)
}
