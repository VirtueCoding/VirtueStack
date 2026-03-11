// Package integration provides end-to-end integration tests for VirtueStack.
// These tests use testcontainers to spin up PostgreSQL and NATS containers.
package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSuite holds all dependencies needed for integration tests.
type TestSuite struct {
	DBPool         *pgxpool.Pool
	NATSConn       *nats.Conn
	JetStream      nats.JetStreamContext
	Logger         *slog.Logger
	JWTSecret      string
	EncryptionKey  string

	// Repositories
	CustomerRepo  *repository.CustomerRepository
	VMRepo        *repository.VMRepository
	NodeRepo      *repository.NodeRepository
	PlanRepo      *repository.PlanRepository
	TemplateRepo  *repository.TemplateRepository
	BackupRepo    *repository.BackupRepository
	WebhookRepo   *repository.WebhookRepository
	IPRepo        *repository.IPRepository
	AdminRepo     *repository.AdminRepository
	TaskRepo      *repository.TaskRepository

	// Services
	AuthService   *services.AuthService
	VMService     *services.VMService
	BackupService *services.BackupService
	WebhookService *services.WebhookService

	// Container references for cleanup
	pgContainer   *postgres.PostgresContainer
	natsContainer *nats.NatsContainer
}

// Test fixture IDs
const (
	TestCustomerID = "00000000-0000-0000-0000-000000000001"
	TestAdminID    = "00000000-0000-0000-0000-000000000002"
	TestPlanID     = "00000000-0000-0000-0000-000000000003"
	TestTemplateID = "00000000-0000-0000-0000-000000000004"
	TestNodeID     = "00000000-0000-0000-0000-000000000005"
	TestVMID       = "00000000-0000-0000-0000-000000000006"
)

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
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
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

	// Run migrations
	migrator, err := migrate.New(
		"file://../../migrations",
		connStr,
	)
	if err != nil {
		logger.Error("failed to create migrator", "error", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		logger.Error("failed to run migrations", "error", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}
	_ = migrator.Close()

	// Create connection pool
	dbPool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		logger.Error("failed to create db pool", "error", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	// Start NATS container
	natsContainer, err := nats.Run(ctx, "nats:2.10-alpine")
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
	natsConn, err := nats.Connect(natsURL)
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
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "VIRTUESTACK_TASKS",
		Subjects: []string{"virtuestack.tasks.>"},
	})
	if err != nil {
		logger.Error("failed to create jetstream stream", "error", err)
	}

	// Generate test secrets
	jwtSecret := crypto.GenerateRandomString(32)
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

	// Initialize services
	suite.AuthService = services.NewAuthService(
		suite.CustomerRepo,
		suite.AdminRepo,
		suite.JWTSecret,
		"virtuestack-test",
		suite.EncryptionKey,
		suite.Logger,
	)

	suite.VMService = services.NewVMService(
		suite.VMRepo,
		suite.NodeRepo,
		suite.IPRepo,
		suite.PlanRepo,
		suite.TemplateRepo,
		suite.TaskRepo,
		nil, // taskPublisher
		nil, // nodeAgentClient
		nil, // ipamService
		suite.EncryptionKey,
		suite.Logger,
	)

	suite.BackupService = services.NewBackupService(
		suite.BackupRepo,
		suite.BackupRepo,
		suite.VMRepo,
		nil, // nodeAgent
		nil, // taskPublisher
		suite.Logger,
	)

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

	// Clean existing test data
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM webhooks WHERE customer_id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM backups WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM ip_addresses WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE customer_id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM customers WHERE id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM admins WHERE id = $1", TestAdminID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM plans WHERE id = $1", TestPlanID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM templates WHERE id = $1", TestTemplateID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM nodes WHERE id = $1", TestNodeID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM sessions WHERE user_id IN ($1, $2)", TestCustomerID, TestAdminID)

	// Create test plan
	_, _ = suite.DBPool.Exec(ctx, `
		INSERT INTO plans (id, name, vcpu, memory_mb, disk_gb, bandwidth_gb, price_cents, is_active, created_at, updated_at)
		VALUES ($1, 'Test Plan', 2, 4096, 50, 1000, 999, true, NOW(), NOW())
	`, TestPlanID)

	// Create test template
	_, _ = suite.DBPool.Exec(ctx, `
		INSERT INTO templates (id, name, os_type, os_version, size_gb, is_active, created_at, updated_at)
		VALUES ($1, 'Ubuntu 22.04', 'linux', 'ubuntu-22.04', 10, true, NOW(), NOW())
	`, TestTemplateID)

	// Create test node
	_, _ = suite.DBPool.Exec(ctx, `
		INSERT INTO nodes (id, hostname, ip_address, status, cpu_cores, memory_mb, disk_gb, created_at, updated_at)
		VALUES ($1, 'test-node-1', '192.168.1.100', 'active', 16, 65536, 1000, NOW(), NOW())
	`, TestNodeID)

	// Create test customer with hashed password
	passwordHash, _ := services.Argon2idParams.HashPassword("TestPassword123!")
	_, _ = suite.DBPool.Exec(ctx, `
		INSERT INTO customers (id, email, password_hash, name, status, created_at, updated_at)
		VALUES ($1, 'test@example.com', $2, 'Test Customer', 'active', NOW(), NOW())
	`, TestCustomerID, passwordHash)

	// Create test admin with hashed password
	adminPasswordHash, _ := services.Argon2idParams.HashPassword("AdminPassword123!")
	encryptedTOTP, _ := crypto.Encrypt("JBSWY3DPEHPK3PXP", suite.EncryptionKey)
	_, _ = suite.DBPool.Exec(ctx, `
		INSERT INTO admins (id, email, password_hash, name, role, totp_enabled, totp_secret_encrypted, created_at)
		VALUES ($1, 'admin@example.com', $2, 'Test Admin', 'admin', true, $3, NOW())
	`, TestAdminID, adminPasswordHash, encryptedTOTP)
}

// TeardownTest cleans up after each test.
func TeardownTest(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// Clean all test data
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM webhooks WHERE customer_id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM backups WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM ip_addresses WHERE vm_id IN (SELECT id FROM vms WHERE customer_id = $1)", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE customer_id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM customers WHERE id = $1", TestCustomerID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM admins WHERE id = $1", TestAdminID)
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM sessions WHERE user_id IN ($1, $2)", TestCustomerID, TestAdminID)
}

// CreateTestVM creates a test VM and returns its ID.
func CreateTestVM(ctx context.Context, customerID, planID, nodeID string) (string, error) {
	vmID := crypto.GenerateUUID()
	macAddr := "52:54:00:" + crypto.GenerateRandomHex(6)

	query := `
		INSERT INTO vms (id, customer_id, node_id, plan_id, hostname, status, vcpu, memory_mb, disk_gb, 
			port_speed_mbps, bandwidth_limit_gb, mac_address, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'test-vm', 'provisioning', 2, 4096, 50, 1000, 1000, $5, NOW(), NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, vmID, customerID, nodeID, planID, macAddr).Scan(&vmID)
	if err != nil {
		return "", fmt.Errorf("creating test vm: %w", err)
	}

	return vmID, nil
}

// CreateTestIP creates a test IP address for a VM.
func CreateTestIP(ctx context.Context, vmID string) (string, error) {
	ipID := crypto.GenerateUUID()
	ipAddress := "192.168.1." + crypto.GenerateRandomDigits(3)

	query := `
		INSERT INTO ip_addresses (id, vm_id, address, type, is_primary, created_at)
		VALUES ($1, $2, $3, 'ipv4', true, NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, ipID, vmID, ipAddress).Scan(&ipID)
	if err != nil {
		return "", fmt.Errorf("creating test ip: %w", err)
	}

	return ipID, nil
}

// CreateTestBackup creates a test backup for a VM.
func CreateTestBackup(ctx context.Context, vmID string) (string, error) {
	backupID := crypto.GenerateUUID()

	query := `
		INSERT INTO backups (id, vm_id, type, status, created_at)
		VALUES ($1, $2, 'full', 'completed', NOW())
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
	secretHash := crypto.HashSHA256("test-webhook-secret")

	query := `
		INSERT INTO webhooks (id, customer_id, url, secret_hash, events, is_active, created_at, updated_at)
		VALUES ($1, $2, 'https://example.com/webhook', $3, ARRAY['vm.created', 'vm.deleted'], true, NOW(), NOW())
		RETURNING id
	`

	err := suite.DBPool.QueryRow(ctx, query, webhookID, customerID, secretHash).Scan(&webhookID)
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