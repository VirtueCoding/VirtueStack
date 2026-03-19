// Package nodeagent provides gRPC handlers for console operations.
// This file contains handlers for VNC and serial console streaming.
package nodeagent

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"libvirt.org/go/libvirt"
)

// StreamVNCConsole establishes a bidirectional VNC console stream.
func (h *grpcHandler) StreamVNCConsole(stream nodeagentpb.NodeAgentService_StreamVNCConsoleServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Read first frame to get VM ID
	firstFrame, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "receiving initial frame: %v", err)
	}

	// Parse VM ID from first frame data
	vmID := string(firstFrame.GetData())
	if vmID == "" {
		return status.Error(codes.InvalidArgument, "first frame must contain vm_id")
	}

	logger := h.server.logger.With("vm_id", vmID, "operation", "vnc-console")

	// Get VNC port from the running domain
	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(vmID))
	if err != nil {
		return status.Errorf(codes.NotFound, "VM not found: %v", err)
	}

	vncPort, err := h.server.getVNCPort(domain)
	domain.Free()
	if err != nil {
		return status.Errorf(codes.Internal, "getting VNC port: %v", err)
	}

	// Connect to the VNC server using configured host or default
	vncHost := h.server.config.VNCHost
	if vncHost == "" {
		vncHost = "127.0.0.1"
	}
	vncAddr := fmt.Sprintf("%s:%d", vncHost, vncPort)
	conn, err := net.DialTimeout("tcp", vncAddr, 5*time.Second)
	if err != nil {
		return status.Errorf(codes.Unavailable, "connecting to VNC: %v", err)
	}
	defer conn.Close()

	logger.Info("VNC console stream established", "vnc_addr", vncAddr)

	errCh := make(chan error, 2)

	// Read from VNC, send to gRPC stream
	go func() {
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("reading from VNC: %w", err)
				} else {
					errCh <- nil
				}
				return
			}
			if err := stream.Send(&nodeagentpb.VNCFrame{Data: buf[:n]}); err != nil {
				errCh <- fmt.Errorf("sending to gRPC stream: %w", err)
				return
			}
		}
	}()

	// Read from gRPC stream, write to VNC
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			frame, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					errCh <- fmt.Errorf("receiving from gRPC stream: %w", err)
				} else {
					errCh <- nil
				}
				return
			}
			if _, err := conn.Write(frame.GetData()); err != nil {
				errCh <- fmt.Errorf("writing to VNC: %w", err)
				return
			}
		}
	}()

	// Wait for either direction to finish
	return <-errCh
}

// StreamSerialConsole establishes a bidirectional serial console stream.
func (h *grpcHandler) StreamSerialConsole(stream nodeagentpb.NodeAgentService_StreamSerialConsoleServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	firstFrame, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "receiving initial frame: %v", err)
	}

	vmID := string(firstFrame.GetData())
	if vmID == "" {
		return status.Error(codes.InvalidArgument, "first frame must contain vm_id")
	}

	logger := h.server.logger.With("vm_id", vmID, "operation", "serial-console")

	domain, err := h.server.libvirtConn.LookupDomainByName(vm.DomainNameFromID(vmID))
	if err != nil {
		return status.Errorf(codes.NotFound, "VM not found: %v", err)
	}
	defer domain.Free()

	// Open a console stream via libvirt
	libvirtStream, err := h.server.libvirtConn.NewStream(0)
	if err != nil {
		return status.Errorf(codes.Internal, "creating libvirt stream: %v", err)
	}
	defer libvirtStream.Free()

	if err := domain.OpenConsole("", libvirtStream, libvirt.DOMAIN_CONSOLE_FORCE); err != nil {
		return status.Errorf(codes.Internal, "opening serial console: %v", err)
	}

	logger.Info("serial console stream established")

	errCh := make(chan error, 2)

	// Read from serial console, send to gRPC stream
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := libvirtStream.Recv(buf)
			if err != nil || n < 0 {
				errCh <- nil
				return
			}
			if err := stream.Send(&nodeagentpb.SerialData{Data: buf[:n]}); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Read from gRPC stream, write to serial console
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			data, err := stream.Recv()
			if err != nil {
				errCh <- nil
				return
			}
			if _, err := libvirtStream.Send(data.GetData()); err != nil {
				errCh <- err
				return
			}
		}
	}()

	return <-errCh
}
