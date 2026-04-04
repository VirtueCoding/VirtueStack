package services

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// evacuationConcurrencyLimit is the maximum number of VMs migrated in parallel
// during node evacuation. Bounded concurrency avoids overwhelming destination
// nodes and the gRPC connection pool.
const evacuationConcurrencyLimit = 10

func (c *NodeAgentGRPCClient) EvacuateNode(ctx context.Context, nodeID string) error {
	c.logger.Info("starting node evacuation", "node_id", nodeID)

	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}
	// Silence unused variable warning; node is fetched to confirm existence.
	_ = node

	if err := c.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusDraining); err != nil {
		return fmt.Errorf("updating node status to draining: %w", err)
	}

	vmFilter := models.VMListFilter{
		NodeID: &nodeID,
		PaginationParams: models.PaginationParams{
			PerPage: models.MaxPerPage,
		},
	}

	vms, _, _, err := c.vmRepo.List(ctx, vmFilter)
	if err != nil {
		return fmt.Errorf("listing VMs on node %s: %w", nodeID, err)
	}

	if len(vms) == 0 {
		c.logger.Info("no VMs to evacuate", "node_id", nodeID)
		return nil
	}

	// Pre-fetch target nodes once to avoid an N+1 query per VM.
	targetNodes, _, _, err := c.nodeRepo.List(ctx, models.NodeListFilter{
		Status: util.StringPtr(models.NodeStatusOnline),
	})
	if err != nil {
		return fmt.Errorf("listing target nodes for evacuation: %w", err)
	}

	c.logger.Info("evacuating VMs from node", "node_id", nodeID, "vm_count", len(vms))

	// evacuateVM tries to place a single VM on any eligible target node.
	evacuateVM := func(vm models.VM) {
		for _, targetNode := range targetNodes {
			if targetNode.ID == nodeID {
				continue
			}

			availCPU := targetNode.TotalVCPU - targetNode.AllocatedVCPU
			availMem := targetNode.TotalMemoryMB - targetNode.AllocatedMemoryMB

			if availCPU < vm.VCPU || availMem < vm.MemoryMB {
				continue
			}

			if err := c.StartVM(ctx, targetNode.ID, vm.ID); err != nil {
				c.logger.Warn("failed to start VM on target node",
					"vm_id", vm.ID,
					"target_node_id", targetNode.ID,
					"error", err)
				continue
			}

			if err := c.vmRepo.UpdateNodeAssignment(ctx, vm.ID, targetNode.ID); err != nil {
				c.logger.Warn("failed to reassign VM to target node",
					"vm_id", vm.ID,
					"target_node_id", targetNode.ID,
					"error", err)
			}

			c.logger.Info("evacuated VM to target node",
				"vm_id", vm.ID,
				"old_node_id", nodeID,
				"new_node_id", targetNode.ID)
			return
		}
		c.logger.Warn("no eligible target node found for VM during evacuation", "vm_id", vm.ID)
	}

	// Use errgroup with a concurrency limit to fan-out VM evacuations.
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(evacuationConcurrencyLimit)

	for _, vm := range vms {
		if vm.Status != models.VMStatusRunning {
			c.logger.Debug("skipping non-running VM during evacuation",
				"vm_id", vm.ID,
				"status", vm.Status)
			continue
		}
		vm := vm // capture loop variable
		eg.Go(func() error {
			evacuateVM(vm)
			return nil // individual VM failures are logged, not propagated
		})
	}

	// Wait for all goroutines; errors are intentionally non-fatal per VM.
	if err := eg.Wait(); err != nil {
		c.logger.Warn("node evacuation worker group returned error", "node_id", nodeID, "error", err)
	}

	c.logger.Info("node evacuation completed", "node_id", nodeID)
	return nil
}

func (c *NodeAgentGRPCClient) MigrateVM(ctx context.Context, sourceNodeID, targetNodeID, vmID string, opts *tasks.MigrateVMOptions) error {
	sourceNode, targetNode, err := c.loadMigrationNodes(ctx, sourceNodeID, targetNodeID)
	if err != nil {
		return err
	}
	conn, err := c.connPool.GetConnection(ctx, sourceNodeID, sourceNode.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to source node %s: %w", sourceNodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.MigrateVM(ctx, &nodeagentpb.MigrateVMRequest{
		VmId:                   vmID,
		DestinationNodeAddress: targetNode.GRPCAddress,
		Live:                   true,
	})
	if err != nil {
		return fmt.Errorf("migrating VM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("migration failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) AbortMigration(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.AbortMigration(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("aborting migration: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("abort migration failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) PostMigrateSetup(ctx context.Context, nodeID, vmID string, bandwidthMbps int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)

	// Build bandwidth limits from bandwidthMbps
	var bandwidth *nodeagentpb.BandwidthLimits
	var isThrottled bool
	var throttleRateKbps int32

	if bandwidthMbps > 0 {
		isThrottled = true
		throttleRateKbps = int32(bandwidthMbps * 1000)
		bandwidth = &nodeagentpb.BandwidthLimits{
			AverageKbps: throttleRateKbps,
			PeakKbps:    throttleRateKbps,
			BurstKb:     throttleRateKbps,
		}
	}

	resp, err := client.PostMigrateSetup(ctx, &nodeagentpb.PostMigrateSetupRequest{
		VmId:             vmID,
		Bandwidth:        bandwidth,
		IsThrottled:      isThrottled,
		ThrottleRateKbps: throttleRateKbps,
	})
	if err != nil {
		return fmt.Errorf("post-migrate setup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("post-migrate setup failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) TransferDisk(ctx context.Context, opts *tasks.DiskTransferOptions) error {
	sourceNode, targetNode, err := c.loadMigrationNodes(ctx, opts.SourceNodeID, opts.TargetNodeID)
	if err != nil {
		return err
	}
	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sourceConn, err := c.connPool.GetConnection(relayCtx, opts.SourceNodeID, sourceNode.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to source node %s: %w", opts.SourceNodeID, err)
	}

	targetConn, err := c.connPool.GetConnection(relayCtx, opts.TargetNodeID, targetNode.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to target node %s: %w", opts.TargetNodeID, err)
	}

	sourceClient := nodeagentpb.NewNodeAgentServiceClient(sourceConn)
	targetClient := nodeagentpb.NewNodeAgentServiceClient(targetConn)
	transferStream, err := sourceClient.TransferDisk(relayCtx, buildTransferDiskRequest(opts, targetNode.GRPCAddress))
	if err != nil {
		return fmt.Errorf("initiating disk transfer: %w", err)
	}

	receiveStream, err := targetClient.ReceiveDisk(relayCtx)
	if err != nil {
		return fmt.Errorf("initiating disk receive: %w", err)
	}

	return transferDiskChunks(cancel, transferStream, receiveStream)
}

func (c *NodeAgentGRPCClient) loadMigrationNodes(ctx context.Context, sourceNodeID, targetNodeID string) (sourceNode, targetNode *models.Node, err error) {
	sourceNode, err = c.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting source node %s: %w", sourceNodeID, err)
	}
	targetNode, err = c.nodeRepo.GetByID(ctx, targetNodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting target node %s: %w", targetNodeID, err)
	}
	return sourceNode, targetNode, nil
}

func buildTransferDiskRequest(opts *tasks.DiskTransferOptions, targetAddress string) *nodeagentpb.TransferDiskRequest {
	return &nodeagentpb.TransferDiskRequest{
		SourceDiskPath:       opts.SourceDiskPath,
		TargetNodeAddress:    targetAddress,
		TargetDiskPath:       opts.TargetDiskPath,
		SnapshotName:         opts.SnapshotName,
		Compress:             opts.Compress,
		StorageBackend:       opts.SourceStorageBackend,
		DiskSizeGb:           int32(opts.DiskSizeGB),
		SourceLvmVolumeGroup: opts.SourceLVMVolumeGroup,
		SourceLvmThinPool:    opts.SourceLVMThinPool,
		TargetLvmVolumeGroup: opts.TargetLVMVolumeGroup,
		TargetLvmThinPool:    opts.TargetLVMThinPool,
		IsDeltaSync:          opts.IsDeltaSync,
	}
}

func relayDiskChunks(source nodeagentpb.NodeAgentService_TransferDiskClient, target nodeagentpb.NodeAgentService_ReceiveDiskClient) error {
	for {
		chunk, err := source.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receiving disk transfer: %w", err)
		}
		if err := target.Send(chunk); err != nil {
			return fmt.Errorf("forwarding disk chunk: %w", err)
		}
	}
}

func transferDiskChunks(
	cancel context.CancelFunc,
	source nodeagentpb.NodeAgentService_TransferDiskClient,
	target nodeagentpb.NodeAgentService_ReceiveDiskClient,
) error {
	receiveResultCh := make(chan error, 1)
	go func() {
		receiveResultCh <- awaitDiskReceive(target)
	}()

	relayResultCh := make(chan error, 1)
	go func() {
		relayResultCh <- relayDiskChunks(source, target)
	}()

	var relayErr error
	var receiveErr error
	relayDone := false
	receiveDone := false

	for !relayDone || !receiveDone {
		select {
		case err := <-receiveResultCh:
			receiveDone = true
			receiveErr = err
			if err != nil {
				cancel()
			}
		case err := <-relayResultCh:
			relayDone = true
			if err != nil {
				relayErr = err
				cancel()
				continue
			}
			if err := target.CloseSend(); err != nil {
				relayErr = fmt.Errorf("closing disk receive stream: %w", err)
				cancel()
			}
		}
	}

	return selectDiskTransferError(relayErr, receiveErr)
}

func awaitDiskReceive(stream nodeagentpb.NodeAgentService_ReceiveDiskClient) error {
	resp := new(nodeagentpb.ReceiveDiskResponse)
	if err := stream.RecvMsg(resp); err != nil {
		return fmt.Errorf("completing disk receive: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("disk receive failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func selectDiskTransferError(relayErr, receiveErr error) error {
	if shouldPreferReceiveError(receiveErr) {
		return receiveErr
	}
	if relayErr != nil {
		return relayErr
	}
	return receiveErr
}

func shouldPreferReceiveError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return false
	}
	return status.Code(err) != codes.Canceled
}

func (c *NodeAgentGRPCClient) PrepareMigratedVM(ctx context.Context, targetNodeID, vmID, diskPath string, vm *models.VM) error {
	node, err := c.nodeRepo.GetByID(ctx, targetNodeID)
	if err != nil {
		return fmt.Errorf("getting target node %s: %w", targetNodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, targetNodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to target node %s: %w", targetNodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.PrepareMigratedVM(ctx, &nodeagentpb.PrepareMigratedVMRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		Hostname:       vm.Hostname,
		Vcpu:           int32(vm.VCPU),
		MemoryMb:       int32(vm.MemoryMB),
		StorageBackend: vm.StorageBackend,
		MacAddress:     vm.MACAddress,
		PortSpeedMbps:  int32(vm.PortSpeedMbps),
		CephPool:       node.CephPool,
		CephMonitors:   c.cephMonitors(),
		CephUser:       c.cephUser(),
		CephSecretUuid: c.cephSecretUUID(),
	})
	if err != nil {
		return fmt.Errorf("preparing migrated VM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("prepare migrated VM failed: %s", resp.GetErrorMessage())
	}
	return nil
}
