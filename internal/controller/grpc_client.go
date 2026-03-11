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
	conns   map[string]*grpc.ClientConn // nodeID -> connection
	mu      sync.RWMutex
	caCert  []byte
	logger  *slog.Logger
	tlsCert string
	tlsKey  string
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
		// Check if connection is still valid
		if conn.GetState().String() != "SHUTDOWN" {
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

// createConnection creates a new gRPC connection with mTLS.
func (nc *NodeClient) createConnection(ctx context.Context, address string) (*grpc.ClientConn, error) {
	// Create TLS credentials
	creds, err := nc.createTLSCredentials()
	if err != nil {
		return nil, fmt.Errorf("creating TLS credentials: %w", err)
	}

	// Configure keepalive
	kaParams := keepalive.ClientParameters{
		Time:    KeepaliveTime,
		Timeout: KeepaliveTimeout,
	}

	// Create dial options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(kaParams),
		grpc.WithBlock(),
		grpc.WithTimeout(DefaultCallTimeout),
	}

	// Dial the connection
	conn, err := grpc.DialContext(ctx, address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", address, err)
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

	// Create TLS config
	tlsConfig := &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: false, // Always verify server certificates
		MinVersion:         tls.VersionTLS12,
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
		conn.Close()
		delete(nc.conns, nodeID)
		nc.logger.Info("removed gRPC connection", "node_id", nodeID)
	}
}

// Close closes all gRPC connections.
func (nc *NodeClient) Close() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	for nodeID, conn := range nc.conns {
		conn.Close()
		nc.logger.Debug("closed gRPC connection", "node_id", nodeID)
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

// InsecureNodeClient creates a NodeClient without mTLS for development.
// WARNING: Do not use in production.
func InsecureNodeClient(logger *slog.Logger) *NodeClient {
	return &NodeClient{
		conns:  make(map[string]*grpc.ClientConn),
		logger: logger.With("component", "grpc-client"),
	}
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

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", address, err)
	}

	nc.conns[nodeID] = conn
	nc.logger.Info("created insecure gRPC connection", "node_id", nodeID, "address", address)

	return conn, nil
}