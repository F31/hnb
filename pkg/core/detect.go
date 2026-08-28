package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type k8sVersionInfo struct {
	GitVersion string `json:"gitVersion"`
	Platform   string `json:"platform"`
}

type apiResourceList struct {
	GroupVersion string `json:"groupVersion"`
	Resources    []struct {
		Name string `json:"name"`
	} `json:"resources"`
}

func DetectDistribution(serverURL string, timeout time.Duration) Distribution {
	client := &http.Client{Timeout: timeout}

	version, err := fetchK8sVersion(client, serverURL)
	if err != nil {
		return DistStandard
	}

	gitVersion := strings.ToLower(version.GitVersion)

	if strings.Contains(gitVersion, "k3s") {
		return DistK3s
	}

	if strings.Contains(gitVersion, "kubeedge") {
		return DistKubeEdge
	}

	hasEdgeApp, err := checkCRDExists(client, serverURL, "edgeapplications")
	if err == nil && hasEdgeApp {
		return DistKubeEdge
	}

	return DistStandard
}

func DetectDistributionFromGitVersion(gitVersion string) Distribution {
	lower := strings.ToLower(gitVersion)

	switch {
	case strings.Contains(lower, "k3s"):
		return DistK3s
	case strings.Contains(lower, "kubeedge"):
		return DistKubeEdge
	default:
		return DistStandard
	}
}

func fetchK8sVersion(client *http.Client, serverURL string) (*k8sVersionInfo, error) {
	url := strings.TrimRight(serverURL, "/") + "/version"
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch version: %w", err)
	}
	defer resp.Body.Close()

	var info k8sVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode version: %w", err)
	}
	return &info, nil
}

var knownCNIPlugins = []struct {
	name      string
	dsName    string
	labelKey  string
	labelVal  string
}{
	{name: "cilium", dsName: "cilium"},
	{name: "calico", dsName: "calico-node"},
	{name: "flannel", dsName: "kube-flannel"},
	{name: "weave", dsName: "weave-net"},
	{name: "kube-ovn", dsName: "kube-ovn"},
	{name: "canal", dsName: "canal"},
	{name: "antrea", dsName: "antrea-agent"},
}

func DetectCNIPlugins(serverURL string, timeout time.Duration) []string {
	client := &http.Client{Timeout: timeout}
	detected := make([]string, 0)

	dss, err := fetchDaemonSets(client, serverURL, "kube-system")
	if err != nil {
		return detected
	}

	for _, plugin := range knownCNIPlugins {
		for _, ds := range dss {
			if strings.EqualFold(ds, plugin.dsName) {
				detected = append(detected, plugin.name)
				break
			}
		}
	}

	return detected
}

func DetectCNIDetails(serverURL string, timeout time.Duration) []CNICapability {
	client := &http.Client{Timeout: timeout}
	names, err := fetchDaemonSets(client, serverURL, "kube-system")
	if err != nil {
		return nil
	}

	var result []CNICapability
	for _, name := range names {
		plugin := findPluginByName(name)
		if plugin == nil {
			continue
		}
		detail, err := fetchDaemonSetDetail(client, serverURL, "kube-system", name)
		if err != nil {
			result = append(result, buildCNICapability(plugin.name, ""))
			continue
		}
		version := extractVersionFromImage(detail)
		result = append(result, buildCNICapability(plugin.name, version))
	}
	return result
}

func findPluginByName(dsName string) *struct {
	name     string
	dsName   string
	labelKey string
	labelVal string
} {
	for _, p := range knownCNIPlugins {
		if strings.EqualFold(dsName, p.dsName) {
			return &p
		}
	}
	return nil
}

func buildCNICapability(pluginName, version string) CNICapability {
	cap := CNICapability{
		Plugin:  pluginName,
		Version: version,
	}
	switch pluginName {
	case "cilium":
		cap.SupportsPolicy = true
		cap.SupportsTrace = true
		cap.SupportsIngress = true
		if compareVersions(version, "1.12.0") >= 0 {
			cap.SupportsHubble = true
			cap.SupportsDualStack = true
		}
	case "calico":
		cap.SupportsPolicy = true
		if compareVersions(version, "3.20.0") >= 0 {
			cap.SupportsDualStack = true
		}
	case "weave":
		cap.SupportsPolicy = true
		cap.SupportsDualStack = true
	case "kube-ovn":
		cap.SupportsPolicy = true
		cap.SupportsDualStack = true
	}
	return cap
}

func extractVersionFromImage(image string) string {
	idx := strings.LastIndex(image, ":")
	if idx < 0 {
		return ""
	}
	version := image[idx+1:]
	return strings.TrimPrefix(version, "v")
}

type daemonSetDetail struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

func fetchDaemonSetDetail(client *http.Client, serverURL, namespace, name string) (string, error) {
	url := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/daemonsets/%s",
		strings.TrimRight(serverURL, "/"), namespace, name)
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch daemonset %s: %w", name, err)
	}
	defer resp.Body.Close()

	var detail daemonSetDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return "", fmt.Errorf("decode daemonset %s: %w", name, err)
	}

	if len(detail.Spec.Template.Spec.Containers) > 0 {
		return detail.Spec.Template.Spec.Containers[0].Image, nil
	}
	return "", nil
}

type daemonSetList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"items"`
}

func fetchDaemonSets(client *http.Client, serverURL, namespace string) ([]string, error) {
	url := fmt.Sprintf("%s/apis/apps/v1/namespaces/%s/daemonsets",
		strings.TrimRight(serverURL, "/"), namespace)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch daemonsets: %w", err)
	}
	defer resp.Body.Close()

	var list daemonSetList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode daemonsets: %w", err)
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.Metadata.Name)
	}
	return names, nil
}

func checkCRDExists(client *http.Client, serverURL, resourceName string) (bool, error) {
	url := fmt.Sprintf("%s/apis/apiextensions.k8s.io/v1/customresourcedefinitions", strings.TrimRight(serverURL, "/"))
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var list struct {
		Items []struct {
			Spec struct {
				Names struct {
					Kind string `json:"kind"`
				} `json:"names"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return false, err
	}

	for _, item := range list.Items {
		if strings.EqualFold(item.Spec.Names.Kind, resourceName) {
			return true, nil
		}
	}
	return false, nil
}