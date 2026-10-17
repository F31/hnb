// Package provision preserves the Provider's provisioning surface while the
// node implementation lives in the shared package. Platform DB/Secret adapters
// remain in internal/provider; the shared executor has no platform store access.
package provision

import shared "github.com/F31/hnb/pkg/provision"

type NodeRole = shared.NodeRole
type NodeSpec = shared.NodeSpec
type SecretReference = shared.SecretReference
type ProvisionSpec = shared.ProvisionSpec
type Result = shared.Result
type AddNodesSpec = shared.AddNodesSpec
type AddNodesResult = shared.AddNodesResult
type StepError = shared.StepError
type SSHRunner = shared.SSHRunner
type SSHConfigFunc = shared.SSHConfigFunc
type RealSSH = shared.RealSSH
type SecretStore = shared.SecretStore
type KeyResolver = shared.KeyResolver
type NodeBootstrapper = shared.NodeBootstrapper
type Option = shared.Option
type Provisioner = shared.Provisioner
type ManagementClient = shared.ManagementClient
type StageEvent = shared.StageEvent
type ProgressObserver = shared.ProgressObserver

const DefaultSSHPort = shared.DefaultSSHPort
const DefaultSSHUser = shared.DefaultSSHUser
const RoleControlPlane = shared.RoleControlPlane
const RoleWorker = shared.RoleWorker
const AdminKubeconfigScript = shared.AdminKubeconfigScript

var New = shared.New
var WithBootstrapper = shared.WithBootstrapper
var WithProgressObserver = shared.WithProgressObserver
var WithManagementClient = shared.WithManagementClient
var NewManagementClient = shared.NewManagementClient
var ControlPlaneConfig = shared.ControlPlaneConfig
var PrereqScript = shared.PrereqScript
var RuntimeConfigScript = shared.RuntimeConfigScript
var InitScript = shared.InitScript
var NetworkScript = shared.NetworkScript
var JoinScript = shared.JoinScript
var CNIScript = shared.CNIScript
var AcceptanceScript = shared.AcceptanceScript
var ParseJoinDetails = shared.ParseJoinDetails
var ParsePrivateKey = shared.ParsePrivateKey
var EncodePrivateKeyForSecret = shared.EncodePrivateKeyForSecret
var CAPIManifests = shared.CAPIManifests
