package controller

import (
	"database/sql"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
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
	storageBackendService := services.NewStorageBackendService(
		storageBackendRepo,
		nodeRepo,
		nodeStorageRepo,
		taskRepo,
		repository.NewAdminBackupScheduleRepository(s.dbPool),
		s.logger,
	)

	s.vmService = services.NewVMService(services.VMServiceConfig{
		VMRepo:            vmRepo,
		NodeRepo:          nodeRepo,
		IPRepo:            ipRepo,
		PlanRepo:          planRepo,
		TemplateRepo:      templateRepo,
		TaskRepo:          taskRepo,
		TaskPublisher:     taskPublisher,
		NodeAgent:         nodeAgentClient,
		IPAMService:       s.ipamService,
		StorageBackendSvc: storageBackendService,
		EncryptionKey:     s.config.EncryptionKey.Value(),
		Logger:            s.logger,
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

	heartbeatChecker := services.NewHeartbeatChecker(nodeRepo, s.logger, services.DefaultHeartbeatCheckerConfig())
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
		NodeAgent:       s.nodeClient,
		JWTSecret:       s.config.JWTSecret.Value(),
		Issuer:          "virtuestack",
		EncryptionKey:   s.config.EncryptionKey.Value(),
		ConsoleBaseURL:  s.config.ConsoleBaseURL,
		ISOStoragePath:  isoStoragePath,
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
		RDNSService:             s.rdnsService,
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
