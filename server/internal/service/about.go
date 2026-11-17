package service

import (
	"context"
	"math"
	"net"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/buildinfo"
	"vgpu/internal/data/prom"

	"github.com/go-kratos/kratos/v2/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/singleflight"
)

const (
	aboutQueryTimeout   = 5 * time.Second
	hamiBuildInfoQuery  = "hami_build_info"
	metricsRefreshQuery = "max(hami_webui_metrics_refresh_last_success_timestamp_seconds)"

	hamiStateOK          = "ok"
	hamiStateMissing     = "missing"
	hamiStateUnavailable = "unavailable"

	hamiSupported   = "supported"
	hamiUnsupported = "unsupported"
	hamiDevelopment = "development"
)

// supportedHAMiMinors lists the HAMi releases this WebUI release is tested with.
var supportedHAMiMinors = []string{"2.10", "2.11"}

// HAMi release images carry their tag; CI and source builds carry a suffix or no tag.
var hamiReleaseVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)\.\d+$`)

type AboutService struct {
	pb.UnimplementedAboutServer
	snapshots   singleflight.Group
	promClient  *prom.Client
	nodeUsecase *biz.NodeUsecase
	cluster     biz.ClusterRepo
	log         *log.Helper
	now         func() time.Time
}

func NewAboutService(promClient *prom.Client, nodeUsecase *biz.NodeUsecase, cluster biz.ClusterRepo, logger log.Logger) *AboutService {
	return &AboutService{
		promClient:  promClient,
		nodeUsecase: nodeUsecase,
		cluster:     cluster,
		log:         log.NewHelper(log.With(logger, "module", "service/about")),
		now:         time.Now,
	}
}

func (s *AboutService) GetAbout(ctx context.Context, _ *pb.GetAboutReq) (*pb.AboutReply, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// All callers of this service inspect the same cluster. Share only an in-flight
	// snapshot; every later refresh starts a new read.
	result := s.snapshots.DoChan("about", func() (any, error) {
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aboutQueryTimeout)
		defer cancel()
		return s.readSnapshot(queryCtx), nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-result:
		return r.Val.(*pb.AboutReply), nil
	}
}

func (s *AboutService) readSnapshot(ctx context.Context) *pb.AboutReply {
	var buildInfo, refresh model.Value
	var promBuild v1.BuildinfoResult
	var serverVersion string
	var buildErr, refreshErr, promErr, serverErr error
	var sources *pb.DeviceSources
	var components biz.ClusterComponents
	var componentErr error
	var targetErr error
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		components, componentErr = s.cluster.Components(ctx)
		sources, targetErr = readDeviceSources(components, componentErr == nil, func(visit func(v1.ActiveTarget)) error {
			return s.promClient.WalkActiveTargets(ctx, visit)
		})
	}()
	go func() {
		defer wg.Done()
		buildInfo, buildErr = s.promClient.Query(ctx, hamiBuildInfoQuery)
	}()
	go func() {
		defer wg.Done()
		refresh, refreshErr = s.promClient.Query(ctx, metricsRefreshQuery)
	}()
	go func() {
		defer wg.Done()
		conn, _ := s.promClient.Conn()
		promBuild, promErr = v1.NewAPI(conn).Buildinfo(ctx)
	}()
	go func() {
		defer wg.Done()
		serverVersion, serverErr = s.cluster.ServerVersion(ctx)
	}()
	nodes, nodeErr := s.nodeUsecase.ListAllNodes(ctx)
	wg.Wait()

	if buildErr != nil {
		s.log.WithContext(ctx).Warnf("read HAMi build info: %v", buildErr)
	}
	if refreshErr != nil {
		s.log.WithContext(ctx).Warnf("read metrics refresh time: %v", refreshErr)
	}
	if promErr != nil {
		s.log.WithContext(ctx).Warnf("read Prometheus version: %v", promErr)
	}
	if targetErr != nil {
		s.log.WithContext(ctx).Warnf("read Prometheus targets: %v", targetErr)
	}
	if nodeErr != nil {
		s.log.WithContext(ctx).Warnf("list nodes: %v", nodeErr)
	}
	if serverErr != nil {
		s.log.WithContext(ctx).Warnf("read Kubernetes version: %v", serverErr)
	}
	if componentErr != nil {
		s.log.WithContext(ctx).Warnf("list HAMi components: %v", componentErr)
	}
	hami := hamiVersions(buildInfo, buildErr, components.HAMi)
	return &pb.AboutReply{
		Webui:             &pb.WebUIBuild{Version: buildinfo.Version, Commit: buildinfo.Commit},
		Hami:              hami,
		Kubelets:          kubeletVersions(nodes),
		KubernetesVersion: serverVersion,
		Prometheus: &pb.PrometheusStatus{
			Reachable:          buildErr == nil || refreshErr == nil || promErr == nil || targetErr == nil,
			MetricsRefreshedAt: refreshedAt(refresh, refreshErr),
			RefreshQueryFailed: refreshErr != nil,
			Version:            promBuild.Version,
		},
		DeviceSources: sources,
		CheckedAt:     s.now().UTC().Format(time.RFC3339),
	}
}

func hamiVersions(value model.Value, err error, components []*biz.HAMiComponent) *pb.HamiVersions {
	reply := &pb.HamiVersions{
		Supported:  slices.Clone(supportedHAMiMinors),
		Versions:   []*pb.HamiVersion{},
		Components: []*pb.HamiComponent{},
	}
	pods := newHAMiPodIndex(components)
	for _, component := range components {
		reply.Components = append(reply.Components, &pb.HamiComponent{
			Name:      component.Name,
			Namespace: component.Namespace,
			Pods:      component.Pods,
			Ready:     component.Ready,
			Nodes:     component.Nodes,
			Restarts:  component.Restarts,
		})
	}
	if err != nil {
		reply.State = hamiStateUnavailable
		return reply
	}
	vector, _ := value.(model.Vector)
	type key struct{ version, revision string }
	groups := map[key]*pb.HamiVersion{}
	for _, sample := range vector {
		labels := sample.Metric
		id := key{string(labels["version"]), string(labels["revision"])}
		group, ok := groups[id]
		if !ok {
			group = &pb.HamiVersion{
				Version:   id.version,
				Revision:  id.revision,
				BuildDate: string(labels["build_date"]),
				Support:   hamiSupport(id.version),
			}
			groups[id] = group
			reply.Versions = append(reply.Versions, group)
		}
		pod := pods.match(labels)
		group.Targets = append(group.Targets, &pb.HamiTarget{
			Job:       string(labels["job"]),
			Instance:  string(labels["instance"]),
			Pod:       string(labels["pod"]),
			Component: pod.component,
			PodUid:    pod.uid,
		})
	}
	for _, group := range reply.Versions {
		slices.SortFunc(group.Targets, func(a, b *pb.HamiTarget) int {
			if c := strings.Compare(a.Job, b.Job); c != 0 {
				return c
			}
			return strings.Compare(a.Instance, b.Instance)
		})
	}
	slices.SortFunc(reply.Versions, func(a, b *pb.HamiVersion) int {
		if c := len(b.Targets) - len(a.Targets); c != 0 {
			return c
		}
		return strings.Compare(a.Version+a.Revision, b.Version+b.Revision)
	})
	reply.State = hamiStateOK
	if len(reply.Versions) == 0 {
		reply.State = hamiStateMissing
	}
	return reply
}

type hamiPodRef struct {
	component string
	uid       string
	ips       []string
}

// hamiPodIndex ties scrape targets to component Pods. Scrape jobs are named by
// whoever configures Prometheus, so only a target's Pod or address counts.
type hamiPodIndex struct {
	byNamespacedName map[string][]hamiPodRef
	byName           map[string][]hamiPodRef
	byIP             map[string][]hamiPodRef
}

func newHAMiPodIndex(components []*biz.HAMiComponent) hamiPodIndex {
	index := hamiPodIndex{byNamespacedName: map[string][]hamiPodRef{}, byName: map[string][]hamiPodRef{}, byIP: map[string][]hamiPodRef{}}
	for _, component := range components {
		for _, member := range component.Members {
			ref := hamiPodRef{component: component.Name, uid: member.UID, ips: member.IPs}
			index.byNamespacedName[member.Namespace+"/"+member.Name] = append(index.byNamespacedName[member.Namespace+"/"+member.Name], ref)
			index.byName[member.Name] = append(index.byName[member.Name], ref)
			for _, ip := range member.IPs {
				index.byIP[ip] = append(index.byIP[ip], ref)
			}
		}
	}
	return index
}

// match returns the one Pod a target belongs to, or nothing when its labels fit
// no Pod or several (same-named Pods, host-network Pods sharing an address).
func (index hamiPodIndex) match(labels model.Metric) hamiPodRef {
	host := string(labels["instance"])
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	candidates := index.byIP[host]
	if pod := string(labels["pod"]); pod != "" {
		candidates = index.byName[pod]
		if namespace := string(labels["namespace"]); namespace != "" {
			candidates = index.byNamespacedName[namespace+"/"+pod]
		}
		if len(candidates) > 1 {
			candidates = slices.DeleteFunc(slices.Clone(candidates), func(ref hamiPodRef) bool { return !slices.Contains(ref.ips, host) })
		}
	}
	if len(candidates) != 1 {
		return hamiPodRef{}
	}
	return candidates[0]
}

func hamiSupport(version string) string {
	match := hamiReleaseVersion.FindStringSubmatch(version)
	if match == nil {
		return hamiDevelopment
	}
	if slices.Contains(supportedHAMiMinors, match[1]+"."+match[2]) {
		return hamiSupported
	}
	return hamiUnsupported
}

func kubeletVersions(nodes []*biz.Node) []*pb.KubeletVersion {
	counts := map[string]int32{}
	for _, node := range nodes {
		if node != nil && node.KubeletVersion != "" {
			counts[node.KubeletVersion]++
		}
	}
	versions := make([]*pb.KubeletVersion, 0, len(counts))
	for version, count := range counts {
		versions = append(versions, &pb.KubeletVersion{Version: version, Nodes: count})
	}
	slices.SortFunc(versions, func(a, b *pb.KubeletVersion) int {
		if a.Nodes != b.Nodes {
			return int(b.Nodes - a.Nodes)
		}
		return strings.Compare(a.Version, b.Version)
	})
	return versions
}

func refreshedAt(value model.Value, err error) string {
	vector, ok := value.(model.Vector)
	if err != nil || !ok || len(vector) == 0 {
		return ""
	}
	seconds := float64(vector[0].Value)
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return ""
	}
	return time.Unix(0, int64(seconds*float64(time.Second))).UTC().Format(time.RFC3339)
}
