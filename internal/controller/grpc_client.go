// Package controller provides the VirtueStack Controller application.
package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// gRPC connection constants.
const (
	// DefaultCallTimeout is the default timeout for gRPC calls.
	DefaultCallTimeout = 30 * time.Second
	// KeepaliveTime is the interval between keepalive pings.
	KeepaliveTime = 10 * time.Second
	// KeepaliveTimeout is the timeout for keepalive ping acknowledgments.
	KeepaliveTimeout = 5 * time.Second
)

// NodeClient manages gRPC connections to Node Agents with mTLS.
type NodeClient struct {
	conns    map[string]*grpc.ClientConn // nodeID -> connection
	mu       sync.RWMutex
	caCert   []byte
	logger   *slog.Logger
	tlsCert  string
	tlsKey   string
	insecure bool
}

// NewNodeClient creates a new NodeClient for managing gRPC connections.
// caCertPath is the path to the CA certificate for verifying node certificates.
func NewNodeClient(caCertPath string, logger *slog.Logger) (*NodeClient, error) {
	// Load CA certificate
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate: %w", err)
	}

	return &NodeClient{
		conns:  make(map[string]*grpc.ClientConn),
		caCert: caCert,
		logger: logger.With("component", "grpc-client"),
	}, nil
}

// NewNodeClientWithCert creates a new NodeClient with client TLS certificate.
// This is used when the controller needs to present a client certificate to nodes.
func NewNodeClientWithCert(caCertPath, clientCertPath, clientKeyPath string, logger *slog.Logger) (*NodeClient, error) {
	nc, err := NewNodeClient(caCertPath, logger)
	if err != nil {
		return nil, err
	}

	nc.tlsCert = clientCertPath
	nc.tlsKey = clientKeyPath

	return nc, nil
}

// GetConnection returns or creates a gRPC connection to a node.
// If a connection already exists for the nodeID, it returns the existing connection.
// Otherwise, it creates a new connection using mTLS.
func (nc *NodeClient) GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
	// Check for existing connection
	nc.mu.RLock()
	conn, exists := nc.conns[nodeID]
	nc.mu.RUnlock()

	if exists {
		// Check if connection is still valid using the typed enum constant
		// rather than a string comparison to avoid silent breakage if the
		// string representation changes between gRPC library versions.
		if conn.GetState() != connectivity.Shutdown {
			return conn, nil
		}
		// Connection is dead, remove it
		nc.mu.Lock()
		delete(nc.conns, nodeID)
		nc.mu.Unlock()
	}

	// Create new connection
	nc.mu.Lock()
	defer nc.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := nc.conns[nodeID]; exists {
		return conn, nil
	}

	conn, err := nc.createConnection(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("creating connection to node %s: %w", nodeID, err)
	}

	nc.conns[nodeID] = conn
	nc.logger.Info("created gRPC connection to node", "node_id", nodeID, "address", address)

	return conn, nil
}

// ReleaseConnection signals that the caller is done using a connection.
// The current implementation validates that the released connection matches the
// pooled connection and logs mismatches, but does not perform any teardown.
//
// TODO: If per-caller reference counting is required in the future, this method
// should decrement a reference count and only remove the connection from the pool
// when the count reaches zero. Until then, connections are kept alive until
// RemoveConnection or Close is called explicitly.
func (nc *NodeClient) ReleaseConnection(nodeID string, conn *grpc.ClientConn) {
	nc.mu.RLock()
	held, exists := nc.conns[nodeID]
	nc.mu.RUnlock()

	if !exists {
		nc.logger.Warn("released connection not found in pool", "node_id", nodeID)
		return
	}
	if held != conn {
		nc.logger.Warn("released connection does not match pooled connection", "node_id", nodeID)
		return
	}
}

// createConnection creates a new gRPC connection with mTLS.
func (nc *NodeClient) createConnection(ctx context.Context, address string) (*grpc.ClientConn, error) {
	creds, err := nc.createTLSCredentials()
	if err != nil {
		return nil, fmt.Errorf("creating TLS credentials: %w", err)
	}

	kaParams := keepalive.ClientParameters{
		Time:    KeepaliveTime,
		Timeout: KeepaliveTimeout,
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(kaParams),
	}

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", address, err)
	}

	return conn, nil
}

// createTLSCredentials creates TLS credentials for mTLS.
func (nc *NodeClient) createTLSCredentials() (credentials.TransportCredentials, error) {
	// Create certificate pool from CA cert
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(nc.caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create TLS config.
	// MinVersion is set to TLS 1.3 to avoid known weaknesses in TLS 1.2
	// (BEAST, CRIME, POODLE, LUCKY13, RC4, CBC-mode cipher suites).
	// If legacy node agents that support only TLS 1.2 must be supported,
	// temporarily lower this to tls.VersionTLS12 during the transition.
	tlsConfig := &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: false, // Always verify server certificates
		MinVersion:         tls.VersionTLS13,
	}

	// Load client certificate if provided
	if nc.tlsCert != "" && nc.tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(nc.tlsCert, nc.tlsKey)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// RemoveConnection closes and removes a node's gRPC connection.
func (nc *NodeClient) RemoveConnection(nodeID string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if conn, exists := nc.conns[nodeID]; exists {
		if err := conn.Close(); err != nil {
			nc.logger.Warn("failed to close gRPC connection",
				"node_id", nodeID,
				"error", err)
		}
		delete(nc.conns, nodeID)
		nc.logger.Info("removed gRPC connection", "node_id", nodeID)
	}
}

// Close closes all gRPC connections.
func (nc *NodeClient) Close() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	for nodeID, conn := range nc.conns {
		if err := conn.Close(); err != nil {
			nc.logger.Warn("failed to close gRPC connection",
				"node_id", nodeID,
				"error", err)
		} else {
			nc.logger.Debug("closed gRPC connection", "node_id", nodeID)
		}
	}

	nc.conns = make(map[string]*grpc.ClientConn)
	nc.logger.Info("closed all gRPC connections")
}

// ConnectionCount returns the number of active connections.
func (nc *NodeClient) ConnectionCount() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return len(nc.conns)
}

// InsecureNodeClient creates a NodeClient without mTLS for development/testing.
func InsecureNodeClient(logger *slog.Logger) *NodeClient {
	return &NodeClient{
		conns:    make(map[string]*grpc.ClientConn),
		logger:   logger.With("component", "grpc-client"),
		insecure: true,
	}
}

func (nc *NodeClient) IsInsecure() bool {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.insecure
}

// GetInsecureConnection creates an insecure connection for development.
func (nc *NodeClient) GetInsecureConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
	nc.mu.RLock()
	conn, exists := nc.conns[nodeID]
	nc.mu.RUnlock()

	if exists {
		return conn, nil
	}

	nc.mu.Lock()
	defer nc.mu.Unlock()

	if conn, exists := nc.conns[nodeID]; exists {
		return conn, nil
	}

	kaParams := keepalive.ClientParameters{
		Time:    KeepaliveTime,
		Timeout: KeepaliveTimeout,
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kaParams),
	}

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", address, err)
	}

	nc.conns[nodeID] = conn
	nc.logger.Info("created insecure gRPC connection", "node_id", nodeID, "address", address)

	return conn, nil
}
