// Package nodeagent provides gRPC handlers for guest agent operations.
// This file contains handlers for QEMU guest agent commands including exec,
// password setting, and filesystem freeze/thaw operations.
package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/guest"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"libvirt.org/go/libvirt"
)

// GuestExecCommand executes a command inside the VM via QEMU guest agent.
func (h *grpcHandler) GuestExecCommand(ctx context.Context, req *nodeagentpb.GuestExecRequest) (*nodeagentpb.GuestExecResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetCommand() == "" {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}

	// Whitelist allowed commands: read-only diagnostic commands only.
	// Removed cat, ls, ip, hostname, whoami to prevent info leakage
	// (file contents, directory listings, network config, usernames).
	allowedCommands := map[string]bool{
		"df": true, "free": true, "uname": true, "date": true, "uptime": true,
	}
	fullCmd := req.GetCommand()
	cmdBase := strings.Split(fullCmd, " ")[0]

	resolvedPath, err := filepath.EvalSymlinks(cmdBase)
	if err != nil {
		return nil, status.Errorf(codes.PermissionDenied, "command path could not be resolved: %v", err)
	}

	allowedPaths := []string{"/usr/bin/", "/bin/"}
	validPath := false
	for _, prefix := range allowedPaths {
		if strings.HasPrefix(resolvedPath, prefix) {
			validPath = true
			cmdBase = strings.TrimPrefix(resolvedPath, prefix)
			break
		}
	}
	if !validPath {
		return nil, status.Errorf(codes.PermissionDenied, "command path must be in /usr/bin or /bin")
	}

	if !allowedCommands[cmdBase] {
		return nil, status.Errorf(codes.PermissionDenied, "command %q is not in the allowed whitelist", cmdBase)
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	// Build the guest-exec command
	args := req.GetArgs()

	execCmd := map[string]interface{}{
		"execute": "guest-exec",
		"arguments": map[string]interface{}{
			"path":           fullCmd,
			"arg":            args,
			"capture-output": true,
		},
	}
	cmdJSON, _ := json.Marshal(execCmd)

	timeout := int(req.GetTimeoutSeconds())
	if timeout <= 0 {
		timeout = 10
	}

	output, err := domain.QemuAgentCommand(string(cmdJSON), libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, uint32(timeout))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "guest exec: %v", err)
	}

	// Parse pid from response
	var execResp struct {
		Return struct {
			PID int `json:"pid"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(output), &execResp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing exec response: %v", err)
	}

	// Get the execution status
	time.Sleep(500 * time.Millisecond)
	statusCmd := fmt.Sprintf(`{"execute":"guest-exec-status","arguments":{"pid":%d}}`, execResp.Return.PID)
	statusOutput, err := domain.QemuAgentCommand(statusCmd, libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, uint32(timeout))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting exec status: %v", err)
	}

	var statusResp struct {
		Return struct {
			Exited   bool   `json:"exited"`
			ExitCode int    `json:"exitcode"`
			OutData  string `json:"out-data"`
			ErrData  string `json:"err-data"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(statusOutput), &statusResp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing status response: %v", err)
	}

	return &nodeagentpb.GuestExecResponse{
		ExitCode: int32(statusResp.Return.ExitCode),
		Stdout:   []byte(statusResp.Return.OutData),
		Stderr:   []byte(statusResp.Return.ErrData),
	}, nil
}

// GuestSetPassword changes a user password inside the VM.
func (h *grpcHandler) GuestSetPassword(ctx context.Context, req *nodeagentpb.GuestPasswordRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	if req.GetUsername() == "" || req.GetPasswordHash() == "" {
		return nil, status.Error(codes.InvalidArgument, "username and password_hash are required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	if err := agent.SetUserPassword(ctx, req.GetUsername(), req.GetPasswordHash()); err != nil {
		return nil, status.Errorf(codes.Internal, "setting password: %v", err)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestFreezeFilesystems freezes all filesystems in the VM.
func (h *grpcHandler) GuestFreezeFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	count, err := agent.FreezeFilesystems(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "freezing filesystems: %v", err)
	}

	h.server.logger.Info("filesystems frozen", "vm_id", req.GetVmId(), "count", count)

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestThawFilesystems unfreezes all filesystems in the VM.
func (h *grpcHandler) GuestThawFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	agent := guest.NewQEMUGuestAgent(domain, h.server.logger)
	count, err := agent.ThawFilesystems(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "thawing filesystems: %v", err)
	}

	h.server.logger.Info("filesystems thawed", "vm_id", req.GetVmId(), "count", count)

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// GuestGetNetworkInterfaces retrieves network interface information from the VM.
func (h *grpcHandler) GuestGetNetworkInterfaces(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.GuestNetworkResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(req.GetVmId()))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	cmd := `{"execute":"guest-network-get-interfaces"}`
	output, err := domain.QemuAgentCommand(cmd, libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT, 10)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "querying network interfaces: %v", err)
	}

	var resp struct {
		Return []struct {
			Name       string `json:"name"`
			HWAddr     string `json:"hardware-address"`
			IPAddrList []struct {
				IPAddr string `json:"ip-address"`
				Prefix int    `json:"ip-address-prefix"`
				Type   string `json:"ip-address-type"`
			} `json:"ip-addresses"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, status.Errorf(codes.Internal, "parsing network response: %v", err)
	}

	var ifaces []*nodeagentpb.GuestNetworkInterface
	for _, iface := range resp.Return {
		protoIface := &nodeagentpb.GuestNetworkInterface{
			Name:       iface.Name,
			MacAddress: iface.HWAddr,
		}
		for _, addr := range iface.IPAddrList {
			ipType := nodeagentpb.IPType_IP_TYPE_IPV4
			if addr.Type == "ipv6" {
				ipType = nodeagentpb.IPType_IP_TYPE_IPV6
			}
			protoIface.IpAddresses = append(protoIface.IpAddresses, &nodeagentpb.IPAddress{
				Ip:     addr.IPAddr,
				Prefix: int32(addr.Prefix),
				Type:   ipType,
			})
		}
		ifaces = append(ifaces, protoIface)
	}

	return &nodeagentpb.GuestNetworkResponse{
		VmId:       req.GetVmId(),
		Interfaces: ifaces,
	}, nil
}
