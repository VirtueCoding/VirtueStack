package services

import (
	"context"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
)

func (c *NodeAgentGRPCClient) CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) (*tasks.SnapshotResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{VmId: vmID, Name: snapshotName})
	if err != nil {
		return nil, fmt.Errorf("calling CreateSnapshot: %w", err)
	}

	return &tasks.SnapshotResponse{
		SnapshotID:      resp.GetSnapshotId(),
		RBDSnapshotName: resp.GetRbdSnapshotName(),
		SizeBytes:       resp.GetSizeBytes(),
	}, nil
}

func (c *NodeAgentGRPCClient) DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{VmId: vmID, SnapshotId: snapshotName})
	if err != nil {
		return fmt.Errorf("calling DeleteSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RevertSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{VmId: vmID, SnapshotId: snapshotName})
	if err != nil {
		return fmt.Errorf("calling RevertSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CloneFromBackup(ctx context.Context, nodeID, vmID, backupSnapshot string, diskGB int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RevertSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{
		VmId:       vmID,
		SnapshotId: backupSnapshot,
	})
	if err != nil {
		return fmt.Errorf("calling RevertSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to clone from backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteDisk(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	vm, err := c.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM %s: %w", vmID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	var diskPath string
	if vm.DiskPath != nil {
		diskPath = *vm.DiskPath
	}
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		VmId:           vmID,
		StorageBackend: vm.StorageBackend,
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete disk for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CloneFromTemplate(ctx context.Context, nodeID, vmID, templateImage, templateSnapshot string, diskGB int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateVM(ctx, &nodeagentpb.CreateVMRequest{
		VmId:                vmID,
		TemplateRbdImage:    templateImage,
		TemplateRbdSnapshot: templateSnapshot,
		DiskGb:              int32(diskGB),
		CephMonitors:        c.cephMonitors(),
		CephUser:            c.cephUser(),
		CephSecretUuid:      c.cephSecretUUID(),
		CephPool:            node.CephPool,
	})
	if err != nil {
		return fmt.Errorf("calling CreateVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to clone template for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GuestFreezeFilesystems(ctx context.Context, nodeID, vmID string) (int, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GuestFreezeFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return 0, fmt.Errorf("calling GuestFreezeFilesystems: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to freeze filesystems for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return 0, nil
}

func (c *NodeAgentGRPCClient) GuestThawFilesystems(ctx context.Context, nodeID, vmID string) (int, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GuestThawFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return 0, fmt.Errorf("calling GuestThawFilesystems: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to thaw filesystems for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return 0, nil
}

func (c *NodeAgentGRPCClient) ProtectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	_, err = client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{
		VmId: vmID,
		Name: snapshotName,
	})
	if err != nil {
		return fmt.Errorf("calling CreateSnapshot for protect: %w", err)
	}
	return nil
}

func (c *NodeAgentGRPCClient) UnprotectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{
		VmId:       vmID,
		SnapshotId: snapshotName,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteSnapshot for unprotect: %w", err)
	}
	_ = resp
	return nil
}

func (c *NodeAgentGRPCClient) CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, targetPool string) (string, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return "", fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	cloneName := fmt.Sprintf("vs-%s-clone-%d", vmID, time.Now().Unix())
	resp, err := client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{
		VmId: vmID,
		Name: cloneName,
	})
	if err != nil {
		return "", fmt.Errorf("calling CreateSnapshot for clone: %w", err)
	}
	return resp.GetRbdSnapshotName(), nil
}

func (c *NodeAgentGRPCClient) CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to create QCOW snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteDiskSnapshot(ctx, &nodeagentpb.DeleteDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete QCOW snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return 0, fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to create QCOW backup snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	// compress and backupPath are not forwarded — see function doc for details.
	return 0, nil
}

func (c *NodeAgentGRPCClient) RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	vm, err := c.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM %s: %w", vmID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.PrepareMigratedVM(ctx, &nodeagentpb.PrepareMigratedVMRequest{
		VmId:           vmID,
		DiskPath:       backupPath,
		Hostname:       vm.Hostname,
		Vcpu:           int32(vm.VCPU),
		MemoryMb:       int32(vm.MemoryMB),
		StorageBackend: "qcow",
		CephPool:       node.CephPool,
		CephMonitors:   c.cephMonitors(),
		CephUser:       c.cephUser(),
		CephSecretUuid: c.cephSecretUUID(),
	})
	if err != nil {
		return fmt.Errorf("calling PrepareMigratedVM for restore: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore QCOW backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	// targetPath is not forwarded — see function doc for details.
	return nil
}

func (c *NodeAgentGRPCClient) RestoreLVMBackup(ctx context.Context, nodeID, vmID, backupFilePath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RestoreLVMBackup(ctx, &nodeagentpb.RestoreLVMBackupRequest{
		VmId:           vmID,
		BackupFilePath: backupFilePath,
	})
	if err != nil {
		return fmt.Errorf("calling RestoreLVMBackup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore LVM backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CreateDiskSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName, storageBackend string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		SnapshotName:   snapshotName,
		StorageBackend: storageBackend,
	})
	if err != nil {
		return fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to create disk snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteDiskSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName, storageBackend string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteDiskSnapshot(ctx, &nodeagentpb.DeleteDiskSnapshotRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		SnapshotName:   snapshotName,
		StorageBackend: storageBackend,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete disk snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CreateLVMBackup(ctx context.Context, nodeID, vmID, snapshotName, backupFilePath string) (int64, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateLVMBackup(ctx, &nodeagentpb.CreateLVMBackupRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		BackupFilePath: backupFilePath,
	})
	if err != nil {
		return 0, fmt.Errorf("calling CreateLVMBackup: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to create LVM backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return resp.GetSizeBytes(), nil
}

func (c *NodeAgentGRPCClient) DeleteLVMBackupFile(ctx context.Context, nodeID, backupPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		StorageBackend: "lvm",
		DiskPath:       backupPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM for LVM backup file cleanup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete LVM backup file %s: %s", backupPath, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		StorageBackend: "qcow",
		DiskPath:       backupPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM for backup file cleanup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete QCOW backup file %s: %s", backupPath, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*tasks.QCOWDiskInfo, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetNodeResources(ctx, &nodeagentpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("calling GetNodeResources: %w", err)
	}

	return &tasks.QCOWDiskInfo{
		DiskPath:    diskPath,
		TotalDiskGB: uint64(resp.GetTotalDiskGb()),
		UsedDiskGB:  uint64(resp.GetUsedDiskGb()),
	}, nil
}

func (c *NodeAgentGRPCClient) BuildTemplateFromISO(ctx context.Context, nodeID string, req *tasks.BuildTemplateFromISORequest) (*tasks.BuildTemplateFromISOResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.BuildTemplateFromISO(ctx, &nodeagentpb.BuildTemplateFromISORequest{
		TemplateName:        req.TemplateName,
		IsoPath:             req.ISOPath,
		IsoUrl:              req.ISOURL,
		OsFamily:            req.OSFamily,
		OsVersion:           req.OSVersion,
		DiskSizeGb:          int32(req.DiskSizeGB),
		MemoryMb:            int32(req.MemoryMB),
		Vcpus:               int32(req.VCPUs),
		StorageBackend:      req.StorageBackend,
		RootPassword:        req.RootPassword,
		CustomInstallConfig: req.CustomInstallConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("build template from ISO: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("template build failed: %s", resp.ErrorMessage)
	}

	return &tasks.BuildTemplateFromISOResponse{
		TemplateRef: resp.TemplateRef,
		SnapshotRef: resp.SnapshotRef,
		SizeBytes:   resp.SizeBytes,
	}, nil
}

func (c *NodeAgentGRPCClient) EnsureTemplateCached(ctx context.Context, nodeID string, req *tasks.EnsureTemplateCachedRequest) (*tasks.EnsureTemplateCachedResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.EnsureTemplateCached(ctx, &nodeagentpb.EnsureTemplateCachedRequest{
		TemplateId:        req.TemplateID,
		TemplateName:      req.TemplateName,
		StorageBackend:    req.StorageBackend,
		SourceUrl:         req.SourceURL,
		ExpectedSizeBytes: req.ExpectedSizeBytes,
		ChecksumSha256:    req.ChecksumSHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure template cached: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("template caching failed: %s", resp.ErrorMessage)
	}

	return &tasks.EnsureTemplateCachedResponse{
		LocalPath:     resp.LocalPath,
		AlreadyCached: resp.AlreadyCached,
		SizeBytes:     resp.SizeBytes,
	}, nil
}
