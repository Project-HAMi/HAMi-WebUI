package service

import (
	"context"
	"strconv"
	"strings"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"

	pb "vgpu/api/v1"
)

// ResourcesService exposes frozen read-only /api/v1/* endpoints by validating
// REST inputs and delegating to the existing domain services.
type ResourcesService struct {
	pb.UnimplementedResourcesServer

	nodes        *NodeService
	cards        *CardService
	containers   *ContainerService
	workloads    *WorkloadService
	scheduling   *SchedulingService
	deviceConfig *DeviceConfigService
	about        *AboutService
}

func NewResourcesService(
	nodes *NodeService,
	cards *CardService,
	containers *ContainerService,
	workloads *WorkloadService,
	scheduling *SchedulingService,
	deviceConfig *DeviceConfigService,
	about *AboutService,
) *ResourcesService {
	return &ResourcesService{
		nodes:        nodes,
		cards:        cards,
		containers:   containers,
		workloads:    workloads,
		scheduling:   scheduling,
		deviceConfig: deviceConfig,
		about:        about,
	}
}

func unavailableService(name string) error {
	return kratoserrors.ServiceUnavailable("SERVICE_UNAVAILABLE", name+" service is not configured")
}

func (s *ResourcesService) ListNodes(ctx context.Context, req *pb.ListNodesRequest) (*pb.NodesReply, error) {
	if s == nil || s.nodes == nil {
		return nil, unavailableService("node")
	}
	isSchedulable := strings.TrimSpace(req.GetIsSchedulable())
	if isSchedulable != "" {
		if _, err := strconv.ParseBool(isSchedulable); err != nil {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "is_schedulable must be a boolean")
		}
	}
	return s.nodes.GetAllNodes(ctx, &pb.GetAllNodesReq{
		Filters: &pb.GetAllNodesReq_Filters{
			Ip:            strings.TrimSpace(req.GetIp()),
			Type:          strings.TrimSpace(req.GetType()),
			IsSchedulable: isSchedulable,
		},
		UseResourceInventory: true,
	})
}

func (s *ResourcesService) GetNode(ctx context.Context, req *pb.GetResourceNodeRequest) (*pb.NodeReply, error) {
	if s == nil || s.nodes == nil {
		return nil, unavailableService("node")
	}
	uid := strings.TrimSpace(req.GetUid())
	if uid == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Node uid is required")
	}
	return s.nodes.GetNode(ctx, &pb.GetNodeReq{
		Uid:                  uid,
		UseResourceInventory: true,
	})
}

func (s *ResourcesService) ListNodeContainers(ctx context.Context, req *pb.ListNodeContainersRequest) (*pb.ContainersReply, error) {
	if s == nil || s.containers == nil {
		return nil, unavailableService("container")
	}
	uid := strings.TrimSpace(req.GetUid())
	nodeName := strings.TrimSpace(req.GetNodeName())
	if uid == "" || nodeName == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Node uid and node_name are required")
	}
	return s.containers.GetAllContainers(ctx, &pb.GetAllContainersReq{
		Filters: &pb.GetAllContainersReq_Filters{
			NodeName: nodeName,
			NodeUid:  uid,
		},
		UseNodeAllocation: true,
	})
}

func (s *ResourcesService) ListDevices(ctx context.Context, req *pb.ListDevicesRequest) (*pb.GPUsReply, error) {
	if s == nil || s.cards == nil {
		return nil, unavailableService("card")
	}
	nodeName := strings.TrimSpace(req.GetNodeName())
	nodeUID := strings.TrimSpace(req.GetNodeUid())
	if nodeUID != "" && nodeName == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Node allocation requires node_name and node_uid")
	}
	provider := strings.TrimSpace(req.GetProvider())
	reply, err := s.cards.GetAllGPUs(ctx, &pb.GetAllGpusReq{
		Filters: &pb.GetAllGpusReq_Filters{
			Uid:      strings.TrimSpace(req.GetId()),
			Type:     strings.TrimSpace(req.GetType()),
			NodeName: nodeName,
			NodeUid:  nodeUID,
			Provider: provider,
		},
	})
	if err != nil {
		return nil, err
	}
	if provider == "" || reply == nil {
		return reply, nil
	}
	filtered := make([]*pb.GPUReply, 0, len(reply.List))
	for _, gpu := range reply.List {
		if gpu.GetVendor() == provider {
			filtered = append(filtered, gpu)
		}
	}
	return &pb.GPUsReply{List: filtered}, nil
}

func (s *ResourcesService) GetDevice(ctx context.Context, req *pb.GetDeviceRequest) (*pb.GPUReply, error) {
	if s == nil || s.cards == nil {
		return nil, unavailableService("card")
	}
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Device id is required")
	}
	return s.cards.GetGPU(ctx, &pb.GetGpuReq{Uid: id})
}

func (s *ResourcesService) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.WorkloadsReply, error) {
	if s == nil || s.workloads == nil {
		return nil, unavailableService("workload")
	}
	if req.GetPage() < 0 || req.GetPageSize() < 0 {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "page and page_size must be non-negative")
	}
	return s.workloads.ListWorkloads(ctx, &pb.ListWorkloadsRequest{
		Filters: &pb.ListWorkloadsRequest_Filters{
			Name:      strings.TrimSpace(req.GetName()),
			NodeName:  strings.TrimSpace(req.GetNodeName()),
			Status:    strings.TrimSpace(req.GetStatus()),
			DeviceId:  strings.TrimSpace(req.GetDeviceId()),
			NodeUid:   strings.TrimSpace(req.GetNodeUid()),
			Namespace: strings.TrimSpace(req.GetNamespace()),
			Priority:  strings.TrimSpace(req.GetPriority()),
			Keyword:   strings.TrimSpace(req.GetKeyword()),
		},
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
	})
}

func (s *ResourcesService) GetContainer(ctx context.Context, req *pb.GetResourceContainerRequest) (*pb.ContainerReply, error) {
	if s == nil || s.containers == nil {
		return nil, unavailableService("container")
	}
	podUID := strings.TrimSpace(req.GetPodUid())
	containerName := strings.TrimSpace(req.GetContainer())
	deviceID := strings.TrimSpace(req.GetDeviceId())
	if podUID == "" || containerName == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "pod_uid and container are required")
	}
	reply, err := s.containers.GetContainer(ctx, &pb.GetContainerReq{
		PodUid:   podUID,
		Name:     containerName,
		DeviceId: deviceID,
	})
	if err != nil {
		return nil, err
	}
	if reply == nil || reply.GetPodUid() == "" || reply.GetName() == "" || (deviceID != "" && reply.GetAllocatedDevices() == 0) {
		return nil, kratoserrors.NotFound("RESOURCE_CONTAINER_NOT_FOUND", "Allocated container was not found")
	}
	return reply, nil
}

func (s *ResourcesService) GetSchedulingPod(ctx context.Context, req *pb.GetResourceSchedulingPodRequest) (*pb.SchedulingPodReply, error) {
	if s == nil || s.scheduling == nil {
		return nil, unavailableService("scheduling")
	}
	return s.scheduling.GetSchedulingPod(ctx, &pb.GetSchedulingPodRequest{
		Namespace: strings.TrimSpace(req.GetNamespace()),
		Name:      strings.TrimSpace(req.GetName()),
		Uid:       strings.TrimSpace(req.GetUid()),
	})
}

func (s *ResourcesService) GetDeviceConfig(ctx context.Context, req *pb.GetDeviceConfigReq) (*pb.DeviceConfigReply, error) {
	if s == nil || s.deviceConfig == nil {
		return nil, unavailableService("device-config")
	}
	if req == nil {
		req = &pb.GetDeviceConfigReq{}
	}
	return s.deviceConfig.GetDeviceConfig(ctx, req)
}

func (s *ResourcesService) GetAbout(ctx context.Context, req *pb.GetAboutReq) (*pb.AboutReply, error) {
	if s == nil || s.about == nil {
		return nil, unavailableService("about")
	}
	if req == nil {
		req = &pb.GetAboutReq{}
	}
	return s.about.GetAbout(ctx, req)
}
