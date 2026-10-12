package service

import (
	"context"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/devicecatalog"
)

type DeviceConfigService struct {
	pb.UnimplementedDeviceConfigServer
	catalog devicecatalog.Source
}

func NewDeviceConfigService(catalog devicecatalog.Source) *DeviceConfigService {
	return &DeviceConfigService{catalog: catalog}
}

func (s *DeviceConfigService) GetDeviceConfig(context.Context, *pb.GetDeviceConfigReq) (*pb.DeviceConfigReply, error) {
	snapshot := s.catalog.Snapshot()
	reply := &pb.DeviceConfigReply{
		State:           string(snapshot.State),
		Namespace:       snapshot.Ref.Namespace,
		Name:            snapshot.Ref.Name,
		Key:             snapshot.Ref.Key,
		ResourceVersion: snapshot.ResourceVersion,
		Issues:          snapshot.Issues,
		AscendModels:    []*pb.AscendModel{},
	}
	// API errors stay in the server log; parse errors describe the operator's own file.
	if snapshot.State == devicecatalog.StateInvalid {
		reply.Reason = snapshot.Reason
	}
	if !snapshot.ObservedAt.IsZero() {
		reply.ObservedAt = snapshot.ObservedAt.Format(time.RFC3339)
	}
	for _, word := range snapshot.AscendCommonWords() {
		model := snapshot.Ascend[word]
		item := &pb.AscendModel{
			CommonWord:         model.CommonWord,
			ChipName:           model.ChipName,
			ResourceName:       model.ResourceName,
			ResourceMemoryName: model.ResourceMemoryName,
			ResourceCoreName:   model.ResourceCoreName,
			MemoryAllocatable:  model.MemoryAllocatable,
			MemoryCapacity:     model.MemoryCapacity,
			AiCore:             model.AICore,
			AiCpu:              model.AICPU,
			SuperPod:           model.PairsModules(),
			Templates:          []*pb.AscendTemplate{},
		}
		for _, template := range model.Templates {
			entry := &pb.AscendTemplate{Name: template.Name, Memory: template.Memory, AiCore: template.AICore, AiCpu: template.AICPU}
			if share, ok := model.ComputeShare(template); ok {
				entry.ComputeShare = &share
			}
			item.Templates = append(item.Templates, entry)
		}
		reply.AscendModels = append(reply.AscendModels, item)
	}
	return reply, nil
}
