package controller

import (
	"database/sql"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/webhooks"
	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/billing/native"
	"github.com/AbuGosok/VirtueStack/internal/controller/billing/whmcs"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	stripePayments "github.com/AbuGosok/VirtueStack/internal/controller/payments/stripe"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
)

// InitializeServices initializes all services and handlers.
// This must be called after SetTaskWorker and SetNodeClient.
func (s *Server) InitializeServices() error {
	// Initialize repositories
	vmRepo := repository.NewVMRepository(s.dbPool)
	nodeRepo := repository.NewNodeRepository(s.dbPool)
	ipRepo := repository.NewIPRepository(s.dbPool)
	planRepo := repository.NewPlanRepository(s.dbPool)
	templateRepo := repository.NewTemplateRepository(s.dbPool)
	templateCacheRepo := repository.NewTemplateCacheRepository(s.dbPool)
	customerRepo := repository.NewCustomerRepository(s.dbPool)
	backupRepo := repository.NewBackupRepository(s.dbPool)
	auditRepo := repository.NewAuditRepository(s.dbPool)
	taskRepo := repository.NewTaskRepository(s.dbPool)
	adminRepo := repository.NewAdminRepository(s.dbPool)
	apiKeyRepo := repository.NewCustomerAPIKeyRepository(s.dbPool)
	webhookRepo := repository.NewWebhookRepository(s.dbPool)
	systemWebhookRepo := repository.NewSystemWebhookRepository(s.dbPool)
	preActionWebhookRepo := repository.NewPreActionWebhookRepository(s.dbPool)
	bandwidthRepo := repository.NewBandwidthRepository(s.dbPool)
	settingsRepo := repository.NewSettingsRepository(s.dbPool)
	isoUploadRepo := repository.NewISOUploadRepository(s.dbPool)
	ssoTokenRepo := repository.NewSSOTokenRepository(s.dbPool)

	s.bandwidthRepo = bandwidthRepo
	s.customerAPIKeyRepo = apiKeyRepo

	// Create task publisher using the worker
	var taskPublisher services.TaskPublisher
	if s.taskWorker != nil {
		taskPublisher = services.NewDefaultTaskPublisher(taskRepo, s.logger)
	}

	var nodeAgentClient services.NodeAgentClient
	var backupNodeAgentClient services.BackupNodeAgentClient
	if s.nodeClient != nil {
		nodeAgentGRPCClient := services.NewNodeAgentGRPCClient(nodeRepo, vmRepo, s.nodeClient, &services.CephConfig{
			Monitors:   s.config.CephMonitors,
			User:       s.config.CephUser,
			SecretUUID: s.config.CephSecretUUID,
		}, s.logger)
		nodeAgentClient = nodeAgentGRPCClient
		backupNodeAgentClient = services.NewBackupNodeAgentAdapter(nodeAgentGRPCClient, vmRepo)
	}

	// Initialize services
	s.authService = services.NewAuthService(
		customerRepo,
		adminRepo,
		auditRepo,
		s.config.JWTSecret.Value(),
		"virtuestack", // issuer
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.ipamService = services.NewIPAMService(ipRepo, nodeRepo, s.logger)

	// Create repositories needed for storage backend and failover services
	failoverRepo := repository.NewFailoverRepository(s.dbPool)
	storageBackendRepo := repository.NewStorageBackendRepository(s.dbPool)
	nodeStorageRepo := repository.NewNodeStorageRepository(s.dbPool)
	systemEventService := services.NewSystemEventService(
		systemWebhookRepo,
		taskPublisher,
		s.logger,
	)

	storageBackendService := services.NewStorageBackendService(
		storageBackendRepo,
		nodeRepo,
		nodeStorageRepo,
		taskRepo,
		repository.NewAdminBackupScheduleRepository(s.dbPool),
		systemEventService,
		s.logger,
	)

	preActionWebhookService := services.NewPreActionWebhookService(preActionWebhookRepo, s.logger)

	// Billing repositories
	billingTxRepo := repository.NewBillingTransactionRepository(s.dbPool)
	billingPaymentRepo := repository.NewBillingPaymentRepository(s.dbPool)
	billingCheckpointRepo := repository.NewBillingCheckpointRepository(s.dbPool)
	exchangeRateRepo := repository.NewExchangeRateRepository(s.dbPool)
	billingInvoiceRepo := repository.NewBillingInvoiceRepository(s.dbPool)

	// Billing services
	billingLedgerService := services.NewBillingLedgerService(services.BillingLedgerServiceConfig{
		TransactionRepo: billingTxRepo,
		Logger:          s.logger,
	})

	exchangeRateService := services.NewExchangeRateService(services.ExchangeRateServiceConfig{
		RateRepo: exchangeRateRepo,
		Logger:   s.logger,
	})

	billingRegistry := billing.NewRegistry("", s.logger)
	if err := billingRegistry.Register(whmcs.NewAdapter()); err != nil {
		return fmt.Errorf("registering WHMCS billing adapter: %w", err)
	}

	// Register native billing adapter when enabled
	if s.config.Billing.Providers.Native.Enabled {
		nativeAdapter := native.NewAdapter(native.AdapterConfig{
			LedgerService: billingLedgerService,
			Logger:        s.logger,
		})
		if err := billingRegistry.Register(nativeAdapter); err != nil {
			return fmt.Errorf("registering native billing adapter: %w", err)
		}

		advisoryLockDB := repository.NewAdvisoryLockDB(s.dbPool)
		s.billingScheduler = services.NewBillingScheduler(services.BillingSchedulerConfig{
			LedgerService:  billingLedgerService,
			CheckpointRepo: billingCheckpointRepo,
			VMRepo:         vmRepo,
			PlanRepo:       planRepo,
			DB:             advisoryLockDB,
			Logger:         s.logger,
		})
	}

	// Payment gateway registry
	paymentRegistry := payments.NewPaymentRegistry()

	// Register Stripe provider if configured
	if s.config.Stripe.SecretKey.Value() != "" {
		stripeProvider := stripePayments.NewProvider(stripePayments.ProviderConfig{
			SecretKey:      s.config.Stripe.SecretKey.Value(),
			WebhookSecret:  s.config.Stripe.WebhookSecret.Value(),
			PublishableKey: s.config.Stripe.PublishableKey,
			Logger:         s.logger,
		})
		if err := stripeProvider.ValidateConfig(); err != nil {
			return fmt.Errorf("stripe config validation: %w", err)
		}
		paymentRegistry.Register("stripe", stripeProvider)
		s.logger.Info("stripe payment provider registered")
	}

	// Payment service
	paymentService := services.NewPaymentService(services.PaymentServiceConfig{
		PaymentRegistry: paymentRegistry,
		LedgerService:   billingLedgerService,
		PaymentRepo:     billingPaymentRepo,
		SettingsRepo:    settingsRepo,
		Logger:          s.logger,
	})

	// Stripe webhook handler
	if s.config.Stripe.SecretKey.Value() != "" {
		s.stripeWebhookHandler = webhooks.NewStripeWebhookHandler(
			paymentService, s.logger,
		)
	}

	// Invoice PDF generator and service
	pdfGenerator := services.NewInvoicePDFGenerator(services.InvoicePDFGeneratorConfig{
		InvoiceRepo:  billingInvoiceRepo,
		SettingsRepo: settingsRepo,
		StoragePath:  s.config.Billing.InvoiceStoragePath,
	})
	billingInvoiceService := services.NewBillingInvoiceService(services.BillingInvoiceServiceConfig{
		InvoiceRepo:     billingInvoiceRepo,
		TransactionRepo: billingTxRepo,
		CustomerRepo:    customerRepo,
		VMRepo:          vmRepo,
		PlanRepo:        planRepo,
		PDFGenerator:    pdfGenerator,
		Logger:          s.logger,
	})
	s.invoiceService = billingInvoiceService

	s.vmService = services.NewVMService(services.VMServiceConfig{
		VMRepo:              vmRepo,
		NodeRepo:            nodeRepo,
		IPRepo:              ipRepo,
		PlanRepo:            planRepo,
		TemplateRepo:        templateRepo,
		TaskRepo:            taskRepo,
		TaskPublisher:       taskPublisher,
		NodeAgent:           nodeAgentClient,
		IPAMService:         s.ipamService,
		StorageBackendSvc:   storageBackendService,
		PreActionWebhookSvc: preActionWebhookService,
		BillingHooks:        billing.NewRegistryHookAdapter(billingRegistry),
		CustomerRepo:        customerRepo,
		EncryptionKey:       s.config.EncryptionKey.Value(),
		Logger:              s.logger,
	})

	s.nodeService = services.NewNodeServiceWithDefaults(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.planService = services.NewPlanService(planRepo, s.logger)

	s.templateService = services.NewTemplateServiceWithTasks(services.TemplateServiceTasksConfig{
		TemplateRepo:      templateRepo,
		TemplateCacheRepo: templateCacheRepo,
		StorageBackends: map[string]services.TemplateStorage{
			s.storage.GetStorageType(): s.storage,
		},
		DefaultBackend: string(s.storage.GetStorageType()),
		NodeRepo:       nodeRepo,
		TaskPublisher:  taskPublisher,
		Logger:         s.logger,
	})

	s.customerService = services.NewCustomerService(customerRepo, auditRepo, s.logger)

	s.backupService = services.NewBackupService(services.BackupServiceConfig{
		BackupRepo:    backupRepo,
		SnapshotRepo:  backupRepo, // Same repo handles snapshots
		VMRepo:        vmRepo,
		NodeAgent:     backupNodeAgentClient,
		TaskPublisher: taskPublisher,
		Logger:        s.logger,
	})

	s.migrationService = services.NewMigrationService(
		vmRepo,
		nodeRepo,
		taskRepo,
		taskPublisher,
		nodeAgentClient,
		storageBackendService,
		s.logger,
	)

	failoverService := services.NewFailoverService(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		auditRepo,
		systemEventService,
		failoverRepo,
		storageBackendRepo,
		nodeStorageRepo,
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.failoverMonitor = services.NewFailoverMonitor(
		nodeRepo,
		failoverService,
		s.logger,
		services.DefaultFailoverMonitorConfig(),
	)

	heartbeatChecker := services.NewHeartbeatChecker(nodeRepo, systemEventService, s.logger, services.DefaultHeartbeatCheckerConfig())
	s.heartbeatChecker = heartbeatChecker

	webhookService := services.NewWebhookService(
		webhookRepo,
		taskPublisher,
		s.logger,
		s.config.EncryptionKey.Value(),
	)

	// Initialize PowerDNS rDNS service if MySQL connection is configured
	if s.config.PowerDNS.MySQLURL != "" {
		var err error
		s.powerDNSDB, err = sql.Open("mysql", s.config.PowerDNS.MySQLURL)
		if err != nil {
			s.logger.Warn("failed to connect to PowerDNS MySQL database", "error", err)
		} else {
			s.rdnsService = services.NewRDNSService(s.powerDNSDB, s.logger)
			s.logger.Info("PowerDNS rDNS service initialized")
		}
	}

	// Initialize handlers
	bandwidthService := services.NewBandwidthService(vmRepo, bandwidthRepo, nil, s.logger)

	s.provisioningHandler = provisioning.NewProvisioningHandler(provisioning.ProvisioningHandlerConfig{
		VMService:        s.vmService,
		AuthService:      s.authService,
		CustomerService:  s.customerService,
		BandwidthService: bandwidthService,
		CustomerRepo:     customerRepo,
		TaskRepo:         taskRepo,
		VMRepo:           vmRepo,
		IPRepo:           ipRepo,
		SSOTokenRepo:     ssoTokenRepo,
		AuditRepo:        auditRepo,
		PlanService:      s.planService,
		JWTSecret:        s.config.JWTSecret.Value(),
		Issuer:           "virtuestack",
		EncryptionKey:    s.config.EncryptionKey.Value(),
		Logger:           s.logger,
	})

	isoStoragePath := s.config.FileStorage.ISOStoragePath
	if isoStoragePath == "" {
		isoStoragePath = defaultISOStoragePath
	}

	s.customerHandler = customer.NewCustomerHandler(customer.CustomerHandlerConfig{
		VMService:       s.vmService,
		BackupService:   s.backupService,
		AuthService:     s.authService,
		TemplateService: s.templateService,
		WebhookService:  webhookService,
		CustomerService: s.customerService,
		BillingLedgerService: billingLedgerService,
		PaymentService:       paymentService,
		VMRepo:          vmRepo,
		NodeRepo:        nodeRepo,
		BackupRepo:      backupRepo,
		TemplateRepo:    templateRepo,
		CustomerRepo:    customerRepo,
		APIKeyRepo:      apiKeyRepo,
		AuditRepo:       auditRepo,
		BandwidthRepo:   bandwidthRepo,
		IPRepo:          ipRepo,
		PlanRepo:        planRepo,
		ISOUploadRepo:   isoUploadRepo,
		SSOTokenRepo:    ssoTokenRepo,
		TaskRepo:        taskRepo,
		RDNSService:     s.rdnsService,
		InvoiceService:  billingInvoiceService,
		NodeAgent:       s.nodeClient,
		JWTSecret:       s.config.JWTSecret.Value(),
		Issuer:          "virtuestack",
		EncryptionKey:   s.config.EncryptionKey.Value(),
		ConsoleBaseURL:  s.config.ConsoleBaseURL,
		ISOStoragePath:  isoStoragePath,
		RegistrationEmailVerification: s.config.RegistrationEmailVerification,
		Logger:          s.logger,
	})

	s.adminHandler = admin.NewAdminHandler(admin.AdminHandlerConfig{
		NodeService:             s.nodeService,
		VMService:               s.vmService,
		MigrationService:        s.migrationService,
		PlanService:             s.planService,
		TemplateService:         s.templateService,
		IPAMService:             s.ipamService,
		CustomerService:         s.customerService,
		BackupService:           s.backupService,
		AuthService:             s.authService,
		BillingLedgerService:    billingLedgerService,
		ExchangeRateService:     exchangeRateService,
		PaymentService:          paymentService,
		AuditRepo:               auditRepo,
		IPRepo:                  ipRepo,
		SettingsRepo:            settingsRepo,
		FailoverRepo:            failoverRepo,
		AdminBackupScheduleRepo: repository.NewAdminBackupScheduleRepository(s.dbPool),
		AdminRepo:               adminRepo,
		StorageBackendRepo:      repository.NewStorageBackendRepository(s.dbPool),
		NodeStorageRepo:         repository.NewNodeStorageRepository(s.dbPool),
		NodeRepo:                nodeRepo,
		VMRepo:                  vmRepo,
		ProvisioningKeyRepo:     repository.NewProvisioningKeyRepository(s.dbPool),
		SystemWebhookRepo:       systemWebhookRepo,
		PreActionWebhookRepo:    preActionWebhookRepo,
		CustomerRepo:            customerRepo,
		RDNSService:             s.rdnsService,
		InvoiceService:          billingInvoiceService,
		JWTSecret:               s.config.JWTSecret.Value(),
		Issuer:                  "virtuestack",
		Logger:                  s.logger,
	})

	notificationPreferenceRepo := repository.NewNotificationPreferenceRepository(s.dbPool)
	notificationEventRepo := repository.NewNotificationEventRepository(s.dbPool)

	notifyService := services.NewNotificationService(
		nil,
		nil,
		notificationPreferenceRepo,
		customerRepo,
		services.NotificationConfig{
			EmailEnabled:    s.config.SMTP.Host != "",
			TelegramEnabled: s.config.Telegram.BotToken != "",
		},
		s.logger,
	)

	s.notifyHandler = customer.NewNotificationsHandler(
		notificationPreferenceRepo,
		notificationEventRepo,
		notifyService,
	)

	// Initialize in-app notification system (SSE hub, repo, service, handlers)
	inAppNotifRepo := repository.NewInAppNotificationRepository(s.dbPool)
	s.sseHub = services.NewSSEHub(s.logger)
	s.inAppNotifService = services.NewInAppNotificationService(services.InAppNotificationServiceConfig{
		Repo:   inAppNotifRepo,
		Hub:    s.sseHub,
		Logger: s.logger,
	})
	authCfg := middleware.AuthConfig{JWTSecret: s.config.JWTSecret.Value(), Issuer: "virtuestack"}
	s.customerInAppNotifHandler = customer.NewInAppNotificationsHandler(
		s.inAppNotifService, s.sseHub, authCfg, s.logger,
	)
	s.adminInAppNotifHandler = admin.NewAdminInAppNotificationsHandler(
		s.inAppNotifService, s.sseHub, authCfg, s.logger,
	)

	// Initialize admin backup schedule service for mass backup campaigns
	s.adminBackupScheduleService = services.NewAdminBackupScheduleService(services.AdminBackupScheduleServiceConfig{
		AdminBackupScheduleRepo: repository.NewAdminBackupScheduleRepository(s.dbPool),
		VMRepo:                  vmRepo,
		BackupRepo:              backupRepo,
		TaskPublisher:           taskPublisher,
		Logger:                  s.logger,
	})

	s.logger.Info("services initialized")

	return nil
}

// GetProvisioningKeyRepo returns the provisioning key repository for route registration.
func (s *Server) GetProvisioningKeyRepo() *repository.ProvisioningKeyRepository {
	return repository.NewProvisioningKeyRepository(s.dbPool)
}

// GetIPAMService returns the IPAM service for task handler dependencies.
func (s *Server) GetIPAMService() tasks.IPAMService {
	return s.ipamService
}
