// Package nodeagent provides gRPC handlers for network operations.
// This file contains handlers for bandwidth monitoring and throttling operations.
package nodeagent

import (
	"context"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetBandwidthUsage retrieves current network bandwidth usage for a VM.
func (h *grpcHandler) GetBandwidthUsage(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.BandwidthUsageResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())

	bytesIn, bytesOut, err := bwManager.GetVMNetworkStats(ctx, domainName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting bandwidth usage: %v", err)
	}

	return &nodeagentpb.BandwidthUsageResponse{
		VmId:    req.GetVmId(),
		RxBytes: int64(bytesIn),
		TxBytes: int64(bytesOut),
	}, nil
}

// SetBandwidthLimit applies a bandwidth cap to a VM's network interface.
func (h *grpcHandler) SetBandwidthLimit(ctx context.Context, req *nodeagentpb.BandwidthLimitRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetLimitMbps() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "limit_mbps must be positive")
	}

	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())
	rateKbps := int(req.GetLimitMbps()) * 1000

	if err := bwManager.ApplyThrottle(ctx, domainName, rateKbps); err != nil {
		return nil, status.Errorf(codes.Internal, "setting bandwidth limit: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// ResetBandwidthCounters resets the bandwidth usage counters for a VM.
func (h *grpcHandler) ResetBandwidthCounters(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	// Remove existing throttle and re-apply to reset counters
	bwManager := h.server.newBandwidthManager()
	domainName := vm.DomainNameFromID(req.GetVmId())

	if err := bwManager.RemoveThrottle(ctx, domainName); err != nil {
		h.server.logger.Warn("failed to remove throttle for counter reset", "error", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}
