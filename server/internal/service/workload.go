package service

import (
	"context"
	"sort"
	"strings"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	workloadStatusPending = "pending"
	workloadStatusAll     = "all"
)

var workloadStatusFilters = []string{workloadStatusPending, biz.ContainerStatusWaiting, biz.ContainerStatusSuccess, "abnormal"}

type WorkloadService struct {
	pb.UnimplementedWorkloadServer
	containers *ContainerService
	scheduling *biz.SchedulingUsecase
}

func NewWorkloadService(containers *ContainerService, scheduling *biz.SchedulingUsecase) *WorkloadService {
	return &WorkloadService{containers: containers, scheduling: scheduling}
}

// ListWorkloads joins allocations and GPU requests from the Pod cache. It never reads Events.
func (s *WorkloadService) ListWorkloads(ctx context.Context, req *pb.ListWorkloadsRequest) (*pb.WorkloadsReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.ListWorkloadsRequest_Filters{}
	}
	allocated, err := s.containers.GetAllContainers(ctx, &pb.GetAllContainersReq{Filters: &pb.GetAllContainersReq_Filters{DeviceId: filters.DeviceId}})
	if err != nil {
		return nil, err
	}
	// Read Pods after allocations so a newer pending snapshot wins.
	pods, err := s.scheduling.ListSchedulingPods(ctx)
	if err != nil {
		return nil, err
	}
	items := mergeWorkloads(allocated.Items, pods)
	// Request-only bound rows get their node UID from the node cache.
	needsNodeUID := false
	for _, item := range items {
		if item.Request != nil && item.NodeName != "" {
			needsNodeUID = true
			break
		}
	}
	if needsNodeUID {
		nodes, err := s.containers.node.ListAllNodes(ctx)
		switch {
		case err == nil:
			uids := make(map[string]string, len(nodes))
			for _, node := range nodes {
				uids[node.Name] = node.Uid
			}
			for _, row := range items {
				if row.Request != nil {
					row.NodeUid = uids[row.NodeName]
				}
			}
		case filters.NodeUid != "":
			// Otherwise the node UID filter would silently drop these rows.
			return nil, err
		}
	}
	filtered := make([]*pb.ContainerReply, 0, len(items))
	counts := map[string]int32{}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !matchesUnifiedWorkloadScope(item, filters) {
			continue
		}
		counts[workloadStatusAll]++
		for _, status := range workloadStatusFilters {
			if matchesWorkloadStatus(item.Status, status) {
				counts[status]++
			}
		}
		if matchesWorkloadStatus(item.Status, filters.Status) {
			filtered = append(filtered, item)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return workloadLess(filtered[i], filtered[j]) })
	result := &pb.WorkloadsReply{Items: []*pb.ContainerReply{}, Total: int32(len(filtered)), StatusCounts: counts}
	page, pageSize := req.GetPage(), req.GetPageSize()
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := int64(page-1) * int64(pageSize)
	if start >= int64(len(filtered)) {
		return result, nil
	}
	end := start + int64(pageSize)
	if end > int64(len(filtered)) {
		end = int64(len(filtered))
	}
	result.Items = filtered[start:end]
	return result, nil
}

func mergeWorkloads(allocated []*pb.ContainerReply, pods []*biz.SchedulingPod) []*pb.ContainerReply {
	items := make([]*pb.ContainerReply, 0, len(allocated)+len(pods))
	bound := make(map[string]bool)
	allocatedIdentities := make(map[string]bool)
	projected := make(map[string]bool)
	unbound := make([]*pb.ContainerReply, 0)
	for _, item := range allocated {
		if item.ContainerKind == "" {
			item.ContainerKind = biz.ContainerKindRegular
		}
		if item.NodeName == "" {
			// A HAMi reservation made before binding.
			unbound = append(unbound, item)
			continue
		}
		items = append(items, item)
		bound[item.PodUid] = true
		allocatedIdentities[workloadIdentity(item.PodUid, item.Name)] = true
	}
	for _, pod := range pods {
		if bound[pod.UID] && pod.NodeName == "" {
			// A known binding wins an overlapping older pending snapshot.
			continue
		}
		scheduling := schedulingPodReply(pod)
		for i, request := range scheduling.Requests {
			if !hasPositiveSchedulingRequest(request) || allocatedIdentities[workloadIdentity(pod.UID, request.Container)] {
				continue
			}
			row := &pb.ContainerReply{
				Name: request.Container, AppName: pod.Name, PodUid: pod.UID,
				Namespace: pod.Namespace, CreateTime: scheduling.CreatedAt,
				NodeName: pod.NodeName, Pending: pod.NodeName == "",
				Status:     workloadStatusPending,
				Scheduling: scheduling, Request: request, ContainerKind: request.ContainerKind,
			}
			if !row.Pending {
				row.Status = normalizedContainerStatus(pod.Requests[i].Status)
				if row.Status == biz.ContainerStatusClosed {
					// A completed container without an allocation holds no GPU.
					continue
				}
				row.StatusDetail = containerStatusDetailReply(pod.Requests[i].StatusDetail)
			}
			items = append(items, row)
			projected[workloadIdentity(pod.UID, request.Container)] = true
		}
	}
	// Keep reservations that no request row represents.
	for _, item := range unbound {
		if !bound[item.PodUid] && !projected[workloadIdentity(item.PodUid, item.Name)] {
			items = append(items, item)
		}
	}
	return items
}

// Container names are unique within a Pod, whatever their kind.
func workloadIdentity(uid, name string) string {
	return uid + "/" + name
}

func hasPositiveSchedulingRequest(request *pb.SchedulingContainerRequest) bool {
	for _, requested := range request.Resources {
		quantity, err := resource.ParseQuantity(requested.Value)
		if err == nil && quantity.Sign() > 0 {
			return true
		}
	}
	return false
}

// matchesUnifiedWorkloadScope applies every filter except status.
func matchesUnifiedWorkloadScope(item *pb.ContainerReply, filters *pb.ListWorkloadsRequest_Filters) bool {
	if !matchesWorkloadName(item.AppName, item.Name, strings.TrimSpace(filters.Name)) ||
		(filters.Namespace != "" && filters.Namespace != item.Namespace) ||
		(filters.NodeName != "" && filters.NodeName != item.NodeName) ||
		(filters.NodeUid != "" && filters.NodeUid != item.NodeUid) {
		return false
	}
	if filters.DeviceId != "" {
		matched := false
		for _, device := range item.DeviceIds {
			matched = matched || strings.HasPrefix(device, filters.DeviceId)
		}
		if !matched {
			return false
		}
	}
	priority := strings.TrimSpace(filters.Priority)
	if priority != "" {
		// HAMi's GPU priority, which request-only rows do not have.
		if item.Request != nil || (priority == "0" && item.Priority == "1") || (priority == "1" && item.Priority != "1") {
			return false
		}
	}
	return true
}

func workloadStatusOrder(status string) int {
	if status == workloadStatusPending {
		// After abnormal (1), before starting (3).
		return 2
	}
	return statusOrder[normalizedContainerStatus(status)]
}

func workloadCreatedAt(item *pb.ContainerReply) time.Time {
	created, _ := time.Parse(time.RFC3339, item.CreateTime)
	return created
}

func workloadLess(left, right *pb.ContainerReply) bool {
	if a, b := workloadStatusOrder(left.Status), workloadStatusOrder(right.Status); a != b {
		return a < b
	}
	if left.Status == workloadStatusPending && right.Status == workloadStatusPending {
		// Longest wait first.
		if a, b := workloadCreatedAt(left), workloadCreatedAt(right); !a.Equal(b) {
			return a.Before(b)
		}
	}
	if left.Namespace != right.Namespace {
		return left.Namespace < right.Namespace
	}
	if left.AppName != right.AppName {
		return left.AppName < right.AppName
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.ContainerKind != right.ContainerKind {
		return left.ContainerKind < right.ContainerKind
	}
	return left.PodUid < right.PodUid
}
