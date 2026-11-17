package biz

import (
	"context"
	"strings"
)

// HAMi chart values of the app.kubernetes.io/component label.
const (
	HAMiScheduler    = "hami-scheduler"
	HAMiDevicePlugin = "hami-device-plugin"
)

// HAMiComponent counts the Pods of one HAMi component in one namespace.
type HAMiComponent struct {
	Name      string
	Namespace string
	Pods      int32
	Ready     int32
	Nodes     int32
	Restarts  int32
	Members   []HAMiPod
}

type HAMiPod struct {
	Ready bool

	UID       string
	Name      string
	Namespace string
	IPs       []string
}

// ClusterComponents is one snapshot of the local Pod cache.
type ClusterComponents struct {
	HAMi    []*HAMiComponent
	Plugins []DevicePluginPod
	// ServiceMonitor targets can identify the Pod but omit the container when its port is undeclared.
	MLUEndpoints map[string]ExporterEndpoint // Pod UID -> metrics port and container image
}

type ExporterEndpoint struct {
	Container string
	Port      int32
	Image     string
}

type DevicePluginPod struct {
	ID    string
	Image string
	Ready bool
}

type ClusterRepo interface {
	ServerVersion(context.Context) (string, error)
	Components(context.Context) (ClusterComponents, error)
}

// ImageReference retains the registry (including its port) and drops the optional digest.
func ImageReference(image string) (repository, tag string) {
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndexByte(image, ':'); i > strings.LastIndexByte(image, '/') {
		tag, image = image[i+1:], image[:i]
	}
	image = strings.TrimPrefix(image, "docker.io/")
	return image, tag
}

// Matches a published repository or the same path under a private registry prefix.
// For locally published images (no namespace), deployment identity is checked by the caller.
func MatchesImageRepository(repo, published string) bool {
	if repo == published || (!strings.Contains(published, "/") && repo == "library/"+published) {
		return true
	}
	registry, rest, ok := strings.Cut(repo, "/")
	if !ok || (registry != "localhost" && !strings.ContainsAny(registry, ".:")) {
		return false
	}
	return rest == published || strings.HasSuffix(rest, "/"+published)
}
