package service

import (
	"context"
	"slices"
	"sort"
	"strings"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"k8s.io/apimachinery/pkg/types"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/provider/metax"
	"vgpu/internal/resourceview"
)

type CardService struct {
	pb.UnimplementedCardServer

	node      *biz.NodeUsecase
	pod       *biz.PodUseCase
	source    resourceview.SnapshotSource
	obsReader resourceview.ObservationReader
}

func NewCardService(node *biz.NodeUsecase, pod *biz.PodUseCase, source *resourceview.ObservedSnapshotSource) *CardService {
	var (
		snapSource resourceview.SnapshotSource
		obsReader  resourceview.ObservationReader
	)
	if source != nil {
		snapSource = source
		obsReader = source.Reader
	}
	return &CardService{node: node, pod: pod, source: snapSource, obsReader: obsReader}
}

func (s *CardService) GetAllGPUs(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllGpusReq_Filters{}
	}
	deviceInfos, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch the container list once. StatisticsByDeviceId re-lists all containers
	// on every call, so calling it per device turned this into an O(devices)
	// re-scan; at large scale (100+ cards) that alone was seconds of work.
	var reservations map[biz.DeviceReservationKey]biz.DeviceReservation
	var scoped *biz.NodeHolders
	var unreadable map[string]bool
	if filters.NodeUid != "" {
		if filters.NodeName == "" {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Node allocation requires nodeName and nodeUid")
		}
		holders, err := s.pod.NodeHolders(ctx, filters.NodeName, filters.NodeUid)
		if err != nil {
			return nil, err
		}
		unreadable = holders.UnreadableDeviceIDs(deviceInfos, filters.NodeName, filters.NodeUid)
		scoped = &holders
	} else {
		containers, err := s.pod.ListAllContainers(ctx)
		if err != nil {
			return nil, err
		}
		reservations = biz.AggregateDeviceReservations(containers)
	}

	var res = &pb.GPUsReply{List: []*pb.GPUReply{}}
	for _, device := range deviceInfos {
		gpu := &pb.GPUReply{}
		nodeName := strings.Trim(filters.NodeName, " ")
		if nodeName != "" && nodeName != device.NodeName {
			continue
		}
		deviceType := strings.Trim(filters.Type, " ")
		if deviceType != "" && deviceType != device.Type {
			continue
		}
		deviceUid := strings.Trim(filters.Uid, " ")
		if deviceUid != "" && deviceUid != device.Id {
			continue
		}
		if filters.NodeUid != "" {
			if filters.NodeUid != device.NodeUid || filters.NodeName != device.NodeName {
				continue
			}
			complete := !scoped.UncertainDeviceIDs[device.AliasId] && !unreadable[device.Id]
			gpu.NodeAllocationComplete = &complete
		}
		gpu.Uuid = device.Id
		gpu.NodeName = device.NodeName
		gpu.Type = device.Type
		gpu.VgpuTotal = device.Count
		gpu.CoreTotal = biz.PhysicalCoreBaselinePerDevice
		gpu.MemoryTotal = device.Devmem
		gpu.NodeUid = device.NodeUid
		gpu.Health = device.Health
		gpu.Mode = device.Mode
		gpu.Unconfigured = device.Unconfigured
		gpu.Vendor = device.Provider
		gpu.MigProfiles = migProfiles(device.MigProfiles)
		if device.RegistrationID != "" && device.RegistrationID != device.Id {
			regID := device.RegistrationID
			gpu.RegistrationId = &regID
		}

		var coreKnown bool
		if scoped != nil {
			reservation := scoped.Reservations[device.AliasId]
			gpu.VgpuUsed, gpu.CoreUsed, gpu.MemoryUsed = reservation.Slots, reservation.Cores, reservation.MemoryMiB
			coreKnown = !reservation.CoreUnknown
			if reservation.HasInit {
				gpu.HasInitReservation = &reservation.HasInit
			}
		} else {
			reservation := reservations[biz.DeviceReservationKey{NodeUID: device.NodeUid, DeviceID: device.AliasId}]
			gpu.VgpuUsed, gpu.CoreUsed, gpu.MemoryUsed = reservation.Slots, reservation.Cores, reservation.MemoryMiB
			coreKnown = !reservation.CoreUnknown
		}
		gpu.CoreUsedKnown = &coreKnown

		res.List = append(res.List, gpu)
	}

	sort.SliceStable(res.List, func(i, j int) bool {
		return res.List[i].Uuid < res.List[j].Uuid
	})
	return res, nil
}

func (s *CardService) GetAllGPUTypes(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	deviceInfos, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	var res = &pb.GPUsReply{List: []*pb.GPUReply{}}
	seenTypes := make(map[string]struct{})

	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllGpusReq_Filters{}
	}
	provider := strings.Trim(filters.Provider, " ")
	for _, device := range deviceInfos {
		if provider != "" && provider != device.Provider {
			continue
		}

		if _, exists := seenTypes[device.Type]; !exists {
			seenTypes[device.Type] = struct{}{}
			gpu := &pb.GPUReply{}
			gpu.Type = device.Type
			res.List = append(res.List, gpu)
		}
	}

	sort.Slice(res.List, func(i, j int) bool {
		return res.List[i].Type < res.List[j].Type
	})
	return res, nil
}

func (s *CardService) GetGPU(ctx context.Context, req *pb.GetGpuReq) (*pb.GPUReply, error) {
	return s.getDevice(ctx, req.GetUid(), "", false)
}

func (s *CardService) getDevice(ctx context.Context, deviceID, nodeUID string, rejectAmbiguous bool) (*pb.GPUReply, error) {
	deviceID = strings.TrimSpace(deviceID)
	nodeUID = strings.TrimSpace(nodeUID)
	if deviceID == "" {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Device uid is required")
	}
	devices, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	var matched []*biz.DeviceInfo
	for _, device := range devices {
		if device.Id != deviceID {
			continue
		}
		if nodeUID != "" && device.NodeUid != nodeUID {
			continue
		}
		matched = append(matched, device)
	}
	if len(matched) == 0 {
		return nil, kratoserrors.NotFound("RESOURCE_GPU_NOT_FOUND", "Accelerator device was not found")
	}
	if rejectAmbiguous && len(matched) > 1 {
		return nil, kratoserrors.Conflict("AMBIGUOUS_DEVICE_ID", "Multiple devices match this id; specify node_uid")
	}
	device := matched[0]
	var (
		vGPU, core, memory, coreUnknownCount int32
		coreKnown                            bool
		nodeAllocationComplete               *bool
		hasInitReservation                   *bool
	)
	if device.AliasId != "" {
		if device.NodeName != "" && device.NodeUid != "" {
			if holders, err := s.pod.NodeHolders(ctx, device.NodeName, device.NodeUid); err == nil {
				res := holders.Reservations[device.AliasId]
				vGPU, core, memory = res.Slots, res.Cores, res.MemoryMiB
				coreUnknownCount = res.CoreUnknownCount
				coreKnown = !res.CoreUnknown
				unreadable := holders.UnreadableDeviceIDs(devices, device.NodeName, device.NodeUid)
				complete := !holders.UncertainDeviceIDs[device.AliasId] && !unreadable[device.Id]
				nodeAllocationComplete = &complete
				if res.HasInit {
					hasInit := res.HasInit
					hasInitReservation = &hasInit
				}
			} else {
				// Transient informer lag on another Pod must not fail device identity or telemetry;
				// fall back to node-scoped ListAllContainers and mark allocations incomplete.
				containers, listErr := s.pod.ListAllContainers(ctx)
				if listErr != nil {
					return nil, listErr
				}
				res := biz.AggregateDeviceReservations(containers)[biz.DeviceReservationKey{
					NodeUID:  device.NodeUid,
					DeviceID: device.AliasId,
				}]
				vGPU, core, memory = res.Slots, res.Cores, res.MemoryMiB
				coreUnknownCount = res.CoreUnknownCount
				coreKnown = !res.CoreUnknown
				complete := false
				nodeAllocationComplete = &complete
			}
		} else {
			containers, err := s.pod.ListAllContainers(ctx)
			if err != nil {
				return nil, err
			}
			res := biz.AggregateDeviceReservations(containers)[biz.DeviceReservationKey{
				NodeUID:  device.NodeUid,
				DeviceID: device.AliasId,
			}]
			vGPU, core, memory = res.Slots, res.Cores, res.MemoryMiB
			coreUnknownCount = res.CoreUnknownCount
			coreKnown = !res.CoreUnknown
		}
	}
	telemetryDeviceID, obs, hasObs := s.devicePhysicalSnapshot(ctx, device)
	reply := &pb.GPUReply{
		Uuid:                   device.Id,
		NodeName:               device.NodeName,
		Type:                   device.Type,
		VgpuTotal:              device.Count,
		CoreTotal:              biz.PhysicalCoreBaselinePerDevice,
		MemoryTotal:            device.Devmem,
		NodeUid:                device.NodeUid,
		Health:                 device.Health,
		Mode:                   device.Mode,
		Unconfigured:           device.Unconfigured,
		Vendor:                 device.Provider,
		MigProfiles:            migProfiles(device.MigProfiles),
		NodeAllocationComplete: nodeAllocationComplete,
		HasInitReservation:     hasInitReservation,
		VgpuUsed:               vGPU,
		CoreUsed:               core,
		CoreUsedKnown:          &coreKnown,
		CoreUnknownCount:       &coreUnknownCount,
		MemoryUsed:             memory,
		TelemetryDeviceId:      telemetryDeviceID,
	}
	if device.RegistrationID != "" && device.RegistrationID != device.Id {
		regID := device.RegistrationID
		reply.RegistrationId = &regID
	}
	if hasObs {
		if util, ok := obs.UtilizationRatio.KnownValue(); ok {
			reply.UtilizationRatio = &util
		}
		if usedQ, physQ := obs.MemoryUsedBytes, obs.MemoryPhysicalBytes; usedQ.State.GetState() == resourceview.Known && physQ.State.GetState() == resourceview.Known && usedQ.Value != nil && physQ.Value != nil && physQ.Value.Sign() > 0 {
			usedBytes := usedQ.Value.Value()
			physBytes := physQ.Value.Value()
			reply.MemoryUsedBytes = &usedBytes
			reply.MemoryPhysicalBytes = &physBytes
		}
		if temp, ok := obs.TemperatureCelsius.KnownValue(); ok {
			reply.TemperatureCelsius = &temp
		}
		if power, ok := obs.PowerWatts.KnownValue(); ok {
			reply.PowerWatts = &power
		}
		if device.Mode == "mig" && reply.MemoryUsedBytes == nil && obs.MigMemoryUsedBytes.State.GetState() == resourceview.Known && obs.MigMemoryUsedBytes.Value != nil {
			mig := &pb.MigInstanceMemory{UsedBytes: obs.MigMemoryUsedBytes.Value.Value()}
			if obs.MigMemoryTotalBytes.State.GetState() == resourceview.Known && obs.MigMemoryTotalBytes.Value != nil {
				total := obs.MigMemoryTotalBytes.Value.Value()
				mig.TotalBytes = &total
			}
			reply.MigInstanceMemory = mig
		}
	}
	return reply, nil
}

func (s *CardService) devicePhysicalSnapshot(ctx context.Context, device *biz.DeviceInfo) (string, resourceview.DeviceObservation, bool) {
	if s == nil || device == nil {
		return "", resourceview.DeviceObservation{}, false
	}
	if s.source != nil {
		snapshot, err := s.source.Snapshot(ctx)
		if err == nil {
			inv := resourceview.GroupPhysicalDevices(snapshot)
			for _, group := range inv.Groups {
				if string(group.NodeUID) != device.NodeUid || len(group.Members) == 0 {
					continue
				}
				for _, member := range group.Members {
					if member.Identity.Node != device.NodeName || member.Identity.Provider != device.Provider {
						continue
					}
					if member.Identity.DeviceID == device.Id || (device.AliasId != "" && (member.Identity.DeviceID == device.AliasId || slices.Contains(member.BindingAliases, device.AliasId))) {
						first := group.Members[0].Identity
						telemetryID, err := resourceview.DeviceID(first.Provider, first.Node, first.DeviceID)
						if err != nil {
							telemetryID = ""
						}
						obs := group.Observation(func(d resourceview.Device) resourceview.DeviceObservation {
							return resourceview.DeviceObservation{
								UtilizationRatio:    d.UtilizationRatio,
								MemoryUsedBytes:     d.MemoryUsedBytes,
								MemoryPhysicalBytes: d.MemoryPhysicalBytes,
								TemperatureCelsius:  d.TemperatureCelsius,
								PowerWatts:          d.PowerWatts,
								MigMemoryUsedBytes:  d.MigMemoryUsedBytes,
								MigMemoryTotalBytes: d.MigMemoryTotalBytes,
							}
						})
						return telemetryID, obs, true
					}
				}
			}
		}
	}
	// Devices omitted from PhysicalInventory (e.g. Cambricon SMLU or native MetaX)
	// still publish identity-scoped telemetry into ObservationReader.
	if s.obsReader != nil && device.NodeUid != "" {
		observations := s.obsReader.DeviceObservations()
		if len(observations) > 0 {
			providerName := device.Provider
			if providerName == metax.MetaxSGPUDevice {
				providerName = biz.MetaxGPUDevice
			}
			uid := types.UID(device.NodeUid)
			for _, candidateID := range []string{device.Id, device.AliasId} {
				if candidateID == "" {
					continue
				}
				if obs, ok := observations[resourceview.DeviceIdentity{
					Provider: providerName,
					Node:     device.NodeName,
					NodeUID:  uid,
					DeviceID: candidateID,
				}]; ok {
					return "", obs, true
				}
			}
		}
	}
	return "", resourceview.DeviceObservation{}, false
}

func migProfiles(profiles []biz.MigProfile) []*pb.MigProfile {
	result := make([]*pb.MigProfile, 0, len(profiles))
	for _, profile := range profiles {
		item := &pb.MigProfile{
			Name:       profile.Name,
			MemoryMb:   profile.MemoryMB,
			SliceCount: profile.SliceCount,
			Core:       profile.Core,
			Placements: make([]*pb.MigPlacement, 0, len(profile.Placements)),
		}
		for _, placement := range profile.Placements {
			item.Placements = append(item.Placements, &pb.MigPlacement{Start: placement.Start, Size: placement.Size})
		}
		result = append(result, item)
	}
	return result
}
