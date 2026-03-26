// Package integration provides end-to-end integration tests for VirtueStack.
// These tests use testcontainers to spin up PostgreSQL and NATS containers.
package integration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/alexedwards/argon2id"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	natsclient "github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	natsmodule "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSuite holds all dependencies needed for integration tests.
type TestSuite struct {
	DBPool        *pgxpool.Pool
	NATSConn      *natsclient.Conn
	JetStream     natsclient.JetStreamContext
	Logger        *slog.Logger
	JWTSecret     string
	EncryptionKey string

	// Repositories
	CustomerRepo *repository.CustomerRepository
	VMRepo       *repository.VMRepository
	NodeRepo     *repository.NodeRepository
	PlanRepo     *repository.PlanRepository
	TemplateRepo *repository.TemplateRepository
	BackupRepo   *repository.BackupRepository
	WebhookRepo  *repository.WebhookRepository
	IPRepo       *repository.IPRepository
	AdminRepo    *repository.AdminRepository
	TaskRepo     *repository.TaskRepository
	AuditRepo    *repository.AuditRepository

	// Services
	AuthService    *services.AuthService
	VMService      *services.VMService
	BackupService  *services.BackupService
	WebhookService *services.WebhookService

	// Container references for cleanup
	pgContainer   *postgres.PostgresContainer
	natsContainer *natsmodule.NATSContainer
}

// Test fixture IDs
const (
	TestCustomerID       = "00000000-0000-0000-0000-000000000001"
	TestAdminID          = "00000000-0000-0000-0000-000000000002"
	TestPlanID           = "00000000-0000-0000-0000-000000000003"
	TestTemplateID       = "00000000-0000-0000-0000-000000000004"
	TestNodeID           = "00000000-0000-0000-0000-000000000005"
	TestVMID             = "00000000-0000-0000-0000-000000000006"
	TestIPSetID          = "00000000-0000-0000-0000-000000000007"
	TestStorageBackendID = "00000000-0000-0000-0000-000000000008"
)

// Test credentials - can be overridden via environment variables
var (
	TestDBUser        = getEnvOrDefault("TEST_DB_USER", "test")
	TestDBPassword    = getEnvOrDefault("TEST_DB_PASSWORD", "")                 // Will be generated if empty
	TestCustomerPass  = getEnvOrDefault("TEST_CUSTOMER_PASSWORD", "")           // Will be generated if empty
	TestAdminPass     = getEnvOrDefault("TEST_ADMIN_PASSWORD", "")              // Will be generated if empty
	TestWebhookSecret = getEnvOrDefault("TEST_WEBHOOK_SECRET", "")              // Will be generated if empty
	TestTOTPSecret    = getEnvOrDefault("TEST_TOTP_SECRET", "JBSWY3DPEHPK3PXP") // Keep default for test stability
	TestVMPassword    = getEnvOrDefault("TEST_VM_PASSWORD", "")                 // Will be generated if empty
	TestWrongPassword = getEnvOrDefault("TEST_WRONG_PASSWORD", "WrongPassword123!")
)

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// init generates random credentials if not provided via environment variables
func init() {
	if TestDBPassword == "" {
		TestDBPassword = mustGenerateRandomToken(16)
	}
	if TestCustomerPass == "" {
		TestCustomerPass = mustGenerateRandomToken(16)
	}
	if TestAdminPass == "" {
		TestAdminPass = mustGenerateRandomToken(16)
	}
	if TestWebhookSecret == "" {
		TestWebhookSecret = mustGenerateRandomToken(32)
	}
	if TestVMPassword == "" {
		TestVMPassword = mustGenerateRandomToken(16)
	}
}

func mustGenerateRandomToken(byteLength int) string {
	token, err := crypto.GenerateRandomToken(byteLength)
	if err != nil {
		panic(err)
	}
	return token
}

func mustGenerateRandomHex(hexChars int) string {
	token, err := crypto.SafeGenerateRandomHex(hexChars)
	if err != nil {
		panic(err)
	}
	return token
}

var suite *TestSuite

// TestMain sets up the test suite and runs all tests.
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Start PostgreSQL container
	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("virtuestack_test"),
		postgres.WithUsername(TestDBUser),
		postgres.WithPassword(TestDBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		logger.Error("failed to start postgres container", "error", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		logger.Error("failed to get postgres connection string", "error", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Run migrations.
	migrator, err := migrate.New(
		"file://../../migrations",
		connStr,
	)
	if err != nil {
		logger.Error("failed to create migrator", "error", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	err = migrator.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		logger.Error("failed to run migrations", "error", err)
		_, _ = migrator.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}
	_, _ = migrator.Close()

	// Create connection pool
	dbPool, err := pgxpool.New(ctx, connStr)

	// Start NATS container
	natsContainer, err := natsmodule.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		logger.Error("failed to start nats container", "error", err)
		dbPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	natsURL, err := natsContainer.ConnectionString(ctx)
	if err != nil {
		logger.Error("failed to get nats connection string", "error", err)
		_ = natsContainer.Terminate(ctx)
		dbPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Connect to NATS
	natsConn, err := natsclient.Connect(natsURL)
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		_ = natsContainer.Terminate(ctx)
		dbPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Create JetStream context
	js, err := natsConn.JetStream()
	if err != nil {
		logger.Error("failed to create jetstream context", "error", err)
		natsConn.Close()
		_ = natsContainer.Terminate(ctx)
		dbPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Create test stream
	_, err = js.AddStream(&natsclient.StreamConfig{
		Name:     "VIRTUESTACK_TASKS",
		Subjects: []string{"virtuestack.tasks.>"},
	})
	if err != nil {
		logger.Error("failed to create jetstream stream", "error", err)
	}

	// Generate test secrets
	jwtSecret := mustGenerateRandomToken(32)
	encryptionKey, err := crypto.GenerateEncryptionKey()
	if err != nil {
		logger.Error("failed to generate encryption key", "error", err)
		natsConn.Close()
		_ = natsContainer.Terminate(ctx)
		dbPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Initialize suite
	suite = &TestSuite{
		DBPool:        dbPool,
		NATSConn:      natsConn,
		JetStream:     js,
		Logger:        logger,
		JWTSecret:     jwtSecret,
		EncryptionKey: encryptionKey,
		pgContainer:   pgContainer,
		natsContainer: natsContainer,
	}

	// Initialize repositories
	suite.CustomerRepo = repository.NewCustomerRepository(dbPool)
	suite.VMRepo = repository.NewVMRepository(dbPool)
	suite.NodeRepo = repository.NewNodeRepository(dbPool)
	suite.PlanRepo = repository.NewPlanRepository(dbPool)
	suite.TemplateRepo = repository.NewTemplateRepository(dbPool)
	suite.BackupRepo = repository.NewBackupRepository(dbPool)
	suite.WebhookRepo = repository.NewWebhookRepository(dbPool)
	suite.IPRepo = repository.NewIPRepository(dbPool)
	suite.AdminRepo = repository.NewAdminRepository(dbPool)
	suite.TaskRepo = repository.NewTaskRepository(dbPool)
	suite.AuditRepo = repository.NewAuditRepository(dbPool)

	// Initialize services
	suite.AuthService = services.NewAuthService(
		suite.CustomerRepo,
		suite.AdminRepo,
		suite.AuditRepo,
		suite.JWTSecret,
		"virtuestack-test",
		suite.EncryptionKey,
		suite.Logger,
	)

	suite.VMService = services.NewVMService(services.VMServiceConfig{
		VMRepo:        suite.VMRepo,
		NodeRepo:      suite.NodeRepo,
		IPRepo:        suite.IPRepo,
		PlanRepo:      suite.PlanRepo,
		TemplateRepo:  suite.TemplateRepo,
		TaskRepo:      suite.TaskRepo,
		TaskPublisher: services.NewDefaultTaskPublisher(suite.TaskRepo, suite.Logger),
		NodeAgent:     nil, // nodeAgentClient
		IPAMService:   nil, // ipamService
		EncryptionKey: suite.EncryptionKey,
		Logger:        suite.Logger,
	})

	suite.BackupService = services.NewBackupService(services.BackupServiceConfig{
		BackupRepo:    suite.BackupRepo,
		SnapshotRepo:  suite.BackupRepo,
		VMRepo:        suite.VMRepo,
		NodeAgent:     nil, // nodeAgent
		TaskPublisher: nil, // taskPublisher
		Logger:        suite.Logger,
	})

	suite.WebhookService = services.NewWebhookService(
		suite.WebhookRepo,
		nil, // taskPublisher
		suite.Logger,
		encryptionKey, // Add encryption key for webhook secret encryption
	)

	// Run tests
	code := m.Run()

	// Cleanup
	natsConn.Close()
	_ = natsContainer.Terminate(ctx)
	dbPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

// SetupTest creates fresh test data before each test.
func SetupTest(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Clean existing test data in correct order (respecting foreign keys)
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM webhook_deliveries WHERE webhook_id IN (SELECT id FROM webhooks WHERE customer_id = $1)", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM webhooks WHERE customer_id = $1", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM backups WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM ip_addresses WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE customer_id = $1", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM customers WHERE id = $1", TestCustomerID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM admins WHERE id = $1", TestAdminID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM plans WHERE id = $1", TestPlanID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM templates WHERE id = $1", TestTemplateID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM nodes WHERE id = $1", TestNodeID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM node_storage WHERE node_id = $1 OR storage_backend_id = $2", TestNodeID, TestStorageBackendID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM storage_backends WHERE id = $1", TestStorageBackendID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM sessions WHERE user_id IN ($1, $2)", TestCustomerID, TestAdminID); err != nil {
		t.Logf("setup cleanup warning: %v", err)
	}

	// Create test plan
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO plans (id, name, slug, vcpu, memory_mb, disk_gb, port_speed_mbps, bandwidth_limit_gb, is_active, created_at)
		VALUES ($1, 'Test Plan', 'test-plan', 2, 4096, 50, 1000, 1000, true, NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestPlanID); err != nil {
		t.Logf("setup plan warning: %v", err)
	}

	// Create test template
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO templates (id, name, os_family, os_version, rbd_image, rbd_snapshot, min_disk_gb, is_active, created_at)
		VALUES ($1, 'Ubuntu 22.04', 'linux', 'ubuntu-22.04', 'vs-templates/ubuntu-22.04', 'snap-init', 10, true, NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestTemplateID); err != nil {
		t.Logf("setup template warning: %v", err)
	}

	// Create test node
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, storage_backend, ceph_pool, ceph_user, ceph_monitors, total_vcpu, total_memory_mb, created_at)
		VALUES ($1, 'test-node-1', '192.168.1.100:50051', '192.168.1.100', 'online', 'ceph', 'vs-vms', 'client.admin', '127.0.0.1:6789', 16, 65536, NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestNodeID); err != nil {
		t.Logf("setup node warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO storage_backends (id, name, type, ceph_pool, ceph_user, ceph_monitors, health_status, created_at, updated_at)
		VALUES ($1, 'ceph-test-node-1', 'ceph', 'vs-vms', 'client.admin', '127.0.0.1:6789', 'healthy', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestStorageBackendID); err != nil {
		t.Logf("setup storage backend warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO node_storage (node_id, storage_backend_id, enabled, preferred, created_at)
		VALUES ($1, $2, true, true, NOW())
		ON CONFLICT (node_id, storage_backend_id) DO NOTHING
	`, TestNodeID, TestStorageBackendID); err != nil {
		t.Logf("setup node_storage warning: %v", err)
	}

	// Create test IP set for IP address tests
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO ip_sets (id, name, network, gateway, ip_version, created_at)
		VALUES ($1, 'test-ip-set', '192.168.1.0/24', '192.168.1.1', 4, NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestIPSetID); err != nil {
		t.Logf("setup ip_set warning: %v", err)
	}

	// Create test customer with hashed password
	passwordHash, _ := argon2id.CreateHash(TestCustomerPass, services.Argon2idParams)
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO customers (id, email, password_hash, name, status, created_at, updated_at)
		VALUES ($1, 'test@example.com', $2, 'Test Customer', 'active', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestCustomerID, passwordHash); err != nil {
		t.Logf("setup customer warning: %v", err)
	}

	// Create test admin with hashed password
	adminPasswordHash, _ := argon2id.CreateHash(TestAdminPass, services.Argon2idParams)
	encryptedTOTP, _ := crypto.Encrypt(TestTOTPSecret, suite.EncryptionKey)
	if _, err := suite.DBPool.Exec(ctx, `
		INSERT INTO admins (id, email, password_hash, name, role, totp_enabled, totp_secret_encrypted, created_at)
		VALUES ($1, 'admin@example.com', $2, 'Test Admin', 'admin', true, $3, NOW())
		ON CONFLICT (id) DO NOTHING
	`, TestAdminID, adminPasswordHash, encryptedTOTP); err != nil {
		t.Logf("setup admin warning: %v", err)
	}
}

// TeardownTest cleans up after each test.
func TeardownTest(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Clean all test data
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM webhooks WHERE customer_id = $1", TestCustomerID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM backups WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM ip_addresses WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE customer_id = $1", TestCustomerID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM node_storage WHERE node_id = $1 OR storage_backend_id = $2", TestNodeID, TestStorageBackendID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM storage_backends WHERE id = $1", TestStorageBackendID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM customers WHERE id = $1", TestCustomerID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM admins WHERE id = $1", TestAdminID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
	if _, err := suite.DBPool.Exec(ctx, "DELETE FROM sessions WHERE user_id IN ($1, $2)", TestCustomerID, TestAdminID); err != nil {
		t.Logf("teardown cleanup warning: %v", err)
	}
}

// CreateTestVM creates a test VM and returns its ID.
func CreateTestVM(ctx context.Context, customerID, planID, nodeID string) (string, error) {
	vmID := crypto.GenerateUUID()
	testHostname := fmt.Sprintf("test-vm-%s", vmID[:8])
	// Generate MAC address in proper format: 52:54:00:XX:XX:XX (QEMU default prefix + random suffix)
	macSuffix := mustGenerateRandomHex(6)
	macAddr := fmt.Sprintf("52:54:00:%s:%s:%s", macSuffix[0:2], macSuffix[2:4], macSuffix[4:6])
	resolvedStorageBackendID := TestStorageBackendID

	// Handle nullable node_id - pass NULL if empty string
	var nodeIDArg interface{}
	if nodeID == "" {
		nodeIDArg = nil
	} else {
		nodeIDArg = nodeID
		if err := suite.DBPool.QueryRow(ctx, `
			SELECT storage_backend_id
			FROM node_storage
			WHERE node_id = $1 AND enabled = true
			ORDER BY preferred DESC, created_at ASC
			LIMIT 1
		`, nodeID).Scan(&resolvedStorageBackendID); err != nil {
			return "", fmt.Errorf("resolving test storage backend: %w", err)
		}
	}

	query := `
		INSERT INTO vms (id, customer_id, node_id, plan_id, hostname, status, vcpu, memory_mb, disk_gb,
			port_speed_mbps, bandwidth_limit_gb, mac_address, storage_backend, storage_backend_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'provisioning', 2, 4096, 50, 1000, 1000, $6, 'ceph', $7, NOW(), NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, vmID, customerID, nodeIDArg, planID, testHostname, macAddr, resolvedStorageBackendID).Scan(&vmID)
	if err != nil {
		return "", fmt.Errorf("creating test vm: %w", err)
	}

	return vmID, nil
}

// CreateTestIP creates a test IP address for a VM.
func CreateTestIP(ctx context.Context, vmID string) (string, error) {
	ipID := crypto.GenerateUUID()
	// Use timestamp-based unique IP to avoid collisions
	// Last two octets are derived from current nanoseconds
	now := time.Now().UnixNano()
	lastOctet := int(now%253 + 2) // Range 2-254, ensures uniqueness within test run
	ipAddress := fmt.Sprintf("192.168.1.%d", lastOctet)

	query := `
		INSERT INTO ip_addresses (id, ip_set_id, vm_id, address, ip_version, is_primary, status, created_at)
		VALUES ($1, $2, $3, $4, 4, true, 'assigned', NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, ipID, TestIPSetID, vmID, ipAddress).Scan(&ipID)
	if err != nil {
		return "", fmt.Errorf("creating test ip: %w", err)
	}

	return ipID, nil
}

// CreateTestBackup creates a test backup for a VM.
func CreateTestBackup(ctx context.Context, vmID string) (string, error) {
	backupID := crypto.GenerateUUID()

	query := `
		INSERT INTO backups (id, vm_id, source, status, created_at)
		VALUES ($1, $2, 'manual', 'completed', NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, backupID, vmID).Scan(&backupID)
	if err != nil {
		return "", fmt.Errorf("creating test backup: %w", err)
	}

	return backupID, nil
}

// CreateTestWebhook creates a test webhook for a customer.
func CreateTestWebhook(ctx context.Context, customerID string) (string, error) {
	webhookID := crypto.GenerateUUID()
	// Encrypt the secret like the real service does (secret_hash stores encrypted secret)
	encryptedSecret, err := crypto.Encrypt(TestWebhookSecret, suite.EncryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypting webhook secret: %w", err)
	}

	query := `
		INSERT INTO webhooks (id, customer_id, url, secret_hash, events, active, created_at, updated_at)
		VALUES ($1, $2, 'https://example.com/webhook', $3, ARRAY['vm.created', 'vm.deleted'], true, NOW(), NOW())
		RETURNING id
	`

	err = suite.DBPool.QueryRow(ctx, query, webhookID, customerID, encryptedSecret).Scan(&webhookID)
	if err != nil {
		return "", fmt.Errorf("creating test webhook: %w", err)
	}

	return webhookID, nil
}

// WaitForTask polls for a task to complete or timeout.
func WaitForTask(ctx context.Context, taskID string, timeout time.Duration) (*models.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for task %s", taskID)
		case <-ticker.C:
			task, err := suite.TaskRepo.GetByID(ctx, taskID)
			if err != nil {
				continue
			}
			if task.Status == "completed" || task.Status == "failed" {
				return task, nil
			}
		}
	}
}

// GetTestSuite returns the test suite instance.
func GetTestSuite() *TestSuite {
	return suite
}
