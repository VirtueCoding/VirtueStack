package services

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
)

func (c *NodeAgentGRPCClient) StartVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.StartVM(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling StartVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to start VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) StopVM(ctx context.Context, nodeID, vmID string, timeoutSeconds int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.StopVM(ctx, &nodeagentpb.StopVMRequest{
		VmId:           vmID,
		TimeoutSeconds: int32(timeoutSeconds),
	})
	if err != nil {
		return fmt.Errorf("calling StopVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to stop VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) ForceStopVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.ForceStopVM(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling ForceStopVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to force stop VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling DeleteVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) ResizeVM(ctx context.Context, nodeID, vmID string, vcpu, memoryMB, diskGB int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.ResizeVM(ctx, &nodeagentpb.ResizeVMRequest{
		VmId:        vmID,
		NewVcpu:     int32(vcpu),
		NewMemoryMb: int32(memoryMB),
		NewDiskGb:   int32(diskGB),
	})
	if err != nil {
		return fmt.Errorf("calling ResizeVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to resize VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GetVMMetrics(ctx context.Context, nodeID, vmID string) (*models.VMMetrics, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetVMMetrics(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return nil, fmt.Errorf("calling GetVMMetrics: %w", err)
	}

	return convertProtoMetrics(resp), nil
}

func (c *NodeAgentGRPCClient) GetVMStatus(ctx context.Context, nodeID, vmID string) (string, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return "", fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetVMStatus(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return "", fmt.Errorf("calling GetVMStatus: %w", err)
	}

	return convertProtoStatus(resp.GetStatus()), nil
}

func (c *NodeAgentGRPCClient) CreateVM(ctx context.Context, nodeID string, req *tasks.CreateVMRequest) (*tasks.CreateVMResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateVM(ctx, &nodeagentpb.CreateVMRequest{
		VmId:                req.VMID,
		Hostname:            req.Hostname,
		Vcpu:                int32(req.VCPU),
		MemoryMb:            int32(req.MemoryMB),
		DiskGb:              int32(req.DiskGB),
		StorageBackend:      req.StorageBackend,
		TemplateFilePath:    req.TemplateFilePath,
		TemplateRbdImage:    req.TemplateRBDImage,
		TemplateRbdSnapshot: req.TemplateRBDSnapshot,
		RootPasswordHash:    req.RootPasswordHash,
		SshPublicKeys:       req.SSHPublicKeys,
		Ipv4Address:         req.IPv4Address,
		Ipv4Gateway:         req.IPv4Gateway,
		Ipv6Address:         req.IPv6Address,
		Ipv6Gateway:         req.IPv6Gateway,
		MacAddress:          req.MACAddress,
		PortSpeedMbps:       int32(req.PortSpeedMbps),
		CephMonitors:        req.CephMonitors,
		CephUser:            req.CephUser,
		CephSecretUuid:      req.CephSecretUUID,
		CephPool:            req.CephPool,
		Nameservers:         req.Nameservers,
	})
	if err != nil {
		return nil, fmt.Errorf("calling CreateVM: %w", err)
	}

	return &tasks.CreateVMResponse{
		DomainName: resp.GetLibvirtDomainName(),
		VNCPort:    resp.GetVncPort(),
	}, nil
}

func (c *NodeAgentGRPCClient) GenerateCloudInit(ctx context.Context, nodeID string, cfg *tasks.CloudInitConfig) (string, error) {
	return fmt.Sprintf("/var/lib/virtuestack/cloud-init/%s.iso", cfg.VMID), nil
}
