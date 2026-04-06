package configartifacts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Command     any      `yaml:"command"`
	Environment any      `yaml:"environment"`
	User        string   `yaml:"user"`
	Volumes     []string `yaml:"volumes"`
}

type swaggerFile struct {
	Paths map[string]map[string]swaggerOperation `yaml:"paths"`
}

type swaggerOperation struct {
	Parameters []swaggerParameter `yaml:"parameters"`
}

type swaggerParameter struct {
	In       string `yaml:"in"`
	Name     string `yaml:"name"`
	Required bool   `yaml:"required"`
}

func TestConfigArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check func(t *testing.T, root string)
	}{
		{
			name: "production compose keeps nats auth enabled",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.prod.yml"))

				natsService, ok := compose.Services["nats"]
				require.True(t, ok, "nats service missing from prod compose")

				command := composeCommandString(t, natsService.Command)
				require.Contains(t, command, "--auth", "prod nats command must require auth")
				require.Contains(t, command, "${NATS_AUTH_TOKEN", "prod nats auth must come from NATS_AUTH_TOKEN")

				controllerService, ok := compose.Services["controller"]
				require.True(t, ok, "controller service missing from prod compose")
				controllerEnv := composeEnvironmentMap(t, controllerService.Environment)
				require.Contains(t, controllerEnv["NATS_URL"], "${NATS_AUTH_TOKEN", "prod controller NATS_URL must include auth token")
				require.Contains(t, controllerEnv["NATS_URL"], "@nats:4222", "prod controller NATS_URL must point at the internal nats service")
			},
		},
		{
			name: "development compose is opt in instead of auto loaded",
			check: func(t *testing.T, root string) {
				_, err := os.Stat(filepath.Join(root, "docker-compose.override.yml"))
				require.Error(t, err)
				require.ErrorIs(t, err, os.ErrNotExist)

				data, err := os.ReadFile(filepath.Join(root, "docker-compose.dev.yml"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, "docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d")
				require.NotContains(t, source, "automatically applies this file")
			},
		},
		{
			name: "make docker targets use explicit prod and dev compose files",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "Makefile"))
				require.NoError(t, err)

				source := string(data)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-build:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.prod\.yml build$`), source)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-up:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.prod\.yml up -d$`), source)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-down:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.prod\.yml down$`), source)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-dev-build:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.dev\.yml build$`), source)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-dev-up:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.dev\.yml up -d$`), source)
				require.Regexp(t, regexp.MustCompile(`(?m)^docker-dev-down:\n\tdocker compose -f docker-compose\.yml -f docker-compose\.dev\.yml down$`), source)
			},
		},
		{
			name: "production compose defaults bundled postgres ssl to disable",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.prod.yml"))

				controllerService, ok := compose.Services["controller"]
				require.True(t, ok, "controller service missing from prod compose")

				controllerEnv := composeEnvironmentMap(t, controllerService.Environment)
				require.Contains(t, controllerEnv["DATABASE_URL"], "sslmode=${DATABASE_SSL_MODE:-disable}")
				require.NotContains(t, controllerEnv["DATABASE_URL"], "sslmode=${DATABASE_SSL_MODE:-require}")
			},
		},
		{
			name: "test compose requires encryption key from env test",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.test.yml"))

				controllerService, ok := compose.Services["controller"]
				require.True(t, ok, "controller service missing from test compose")
				controllerEnv := composeEnvironmentMap(t, controllerService.Environment)
				require.Equal(t,
					"${ENCRYPTION_KEY:?ENCRYPTION_KEY must be set in .env.test}",
					controllerEnv["ENCRYPTION_KEY"],
				)
			},
		},
		{
			name: "test compose postgres does not auto-seed before migrations",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.test.yml"))

				postgresService, ok := compose.Services["postgres"]
				require.True(t, ok, "postgres service missing from test compose")
				require.NotContains(t,
					strings.Join(postgresService.Volumes, "\n"),
					"/docker-entrypoint-initdb.d/99_test_seed.sql",
				)
			},
		},
		{
			name: "setup e2e generates hex encryption key",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				require.Regexp(t, regexp.MustCompile(`ENCRYPTION_KEY=\$\((generate_random_hex 32|openssl rand -hex 32)\)`), string(data))
			},
		},
		{
			name: "setup e2e restricts env test permissions",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				require.Contains(t, string(data), `chmod 600 "${PROJECT_ROOT}/.env.test"`)
			},
		},
		{
			name: "setup e2e uses test compose overlay and external migration runner",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				script := string(data)
				require.Contains(t, script, "docker-compose.test.yml")
				require.Contains(t, script, "migrate/migrate")
				require.NotContains(t, script, "docker compose exec -T controller migrate")
			},
		},
		{
			name: "setup e2e seeds from mounted migrations after migration runner",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				script := string(data)
				require.Contains(t, script, "-f /docker-entrypoint-initdb.d/migrations/test_seed.sql")
				require.NotContains(t, script, "/docker-entrypoint-initdb.d/../migrations/test_seed.sql")
			},
		},
		{
			name: "storage repositories use migrated node storage table",
			check: func(t *testing.T, root string) {
				migrationData, err := os.ReadFile(filepath.Join(root, "migrations", "000055_storage_backend_registry.up.sql"))
				require.NoError(t, err)
				migration := string(migrationData)
				require.Contains(t, migration, "CREATE TABLE node_storage (")
				require.NotContains(t, migration, "CREATE TABLE node_storage_backends")

				repoPaths := []string{
					filepath.Join(root, "internal", "controller", "repository", "node_storage_repo.go"),
					filepath.Join(root, "internal", "controller", "repository", "storage_backend_repo.go"),
				}

				for _, repoPath := range repoPaths {
					repoData, err := os.ReadFile(repoPath)
					require.NoError(t, err)
					source := string(repoData)

					require.Contains(t, source, "node_storage")
					require.NotContains(t, source, "node_storage_backends")
				}
			},
		},
		{
			name: "task schema and reads include progress message and updated at",
			check: func(t *testing.T, root string) {
				progressMigrationData, err := os.ReadFile(filepath.Join(root, "migrations", "000060_tasks_progress_message.up.sql"))
				require.NoError(t, err)
				require.Contains(t, string(progressMigrationData), "ADD COLUMN IF NOT EXISTS progress_message TEXT")

				retryMigrationData, err := os.ReadFile(filepath.Join(root, "migrations", "000067_tasks_retry_count.up.sql"))
				require.NoError(t, err)
				require.Contains(t, string(retryMigrationData), "ADD COLUMN IF NOT EXISTS retry_count")

				updatedAtFound := false
				migrationFiles, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
				require.NoError(t, err)
				for _, migrationPath := range migrationFiles {
					data, readErr := os.ReadFile(migrationPath)
					require.NoError(t, readErr)
					if strings.Contains(string(data), "ADD COLUMN IF NOT EXISTS updated_at") &&
						strings.Contains(string(data), "ALTER TABLE tasks") {
						updatedAtFound = true
						break
					}
				}
				require.True(t, updatedAtFound, "tasks.updated_at migration missing")

				repoData, err := os.ReadFile(filepath.Join(root, "internal", "controller", "repository", "task_repo.go"))
				require.NoError(t, err)
				repoSource := string(repoData)
				require.Contains(t, repoSource, "progress_message")
				require.Contains(t, repoSource, "updated_at")

				modelData, err := os.ReadFile(filepath.Join(root, "internal", "controller", "models", "task.go"))
				require.NoError(t, err)
				modelSource := string(modelData)
				require.Contains(t, modelSource, "ProgressMessage")
				require.Contains(t, modelSource, "UpdatedAt")
			},
		},
		{
			name: "customer vm page resyncs search term from url changes",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "webui", "customer", "app", "vms", "page.tsx"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, "getURLSyncedSearchTerm(currentSearchTerm, searchFromUrl)")
				require.Regexp(t,
					regexp.MustCompile(`useEffect\(\(\)\s*=>\s*{\s*setSearchTerm\(\(currentSearchTerm\)\s*=>`),
					source,
				)
				require.Contains(t, source, "}, [searchFromUrl]);")
			},
		},
		{
			name: "customer billing validates top up redirects before navigation",
			check: func(t *testing.T, root string) {
				helperData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "lib", "billing-redirect.ts"))
				require.NoError(t, err)

				helper := string(helperData)
				require.Contains(t, helper, "new URL(")
				require.Contains(t, helper, `parsed.protocol !== "https:"`)

				pageData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "app", "billing", "page.tsx"))
				require.NoError(t, err)

				page := string(pageData)
				require.Contains(t, page, "getSafeTopUpRedirectURL")
				require.NotContains(t, page, "window.location.href = data.payment_url")
			},
		},
		{
			name: "billing modules accept controller webhook signature header contract",
			check: func(t *testing.T, root string) {
				whmcsWebhookData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "webhook.php"))
				require.NoError(t, err)
				whmcsWebhook := string(whmcsWebhookData)
				require.Contains(t, whmcsWebhook, "HTTP_X_WEBHOOK_SIGNATURE")
				require.Contains(t, whmcsWebhook, "HTTP_X_VIRTUESTACK_SIGNATURE")

				whmcsHelperData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "lib", "shared_functions.php"))
				require.NoError(t, err)
				require.Contains(t, string(whmcsHelperData), "X-Webhook-Signature")

				blestaWebhookData, err := os.ReadFile(filepath.Join(root, "modules", "blesta", "virtuestack", "webhook.php"))
				require.NoError(t, err)
				blestaWebhook := string(blestaWebhookData)
				require.Contains(t, blestaWebhook, "HTTP_X_WEBHOOK_SIGNATURE")
				require.Contains(t, blestaWebhook, "HTTP_X_VIRTUESTACK_SIGNATURE")

				blestaHelperData, err := os.ReadFile(filepath.Join(root, "modules", "blesta", "virtuestack", "lib", "VirtueStackHelper.php"))
				require.NoError(t, err)
				blestaHelper := string(blestaHelperData)
				require.Contains(t, blestaHelper, "normalizeWebhookSignature")
				require.Contains(t, blestaHelper, "hash_hmac('sha256', $body, $secret)")
			},
		},
		{
			name: "setup e2e keeps test ssl key private",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				script := string(data)
				require.Contains(t, script, `chmod 600 "${ssl_dir}/key.pem"`)
				require.NotContains(t, script, `chmod 644 "${ssl_dir}/key.pem"`)
			},
		},
		{
			name: "compose does not force nginx container to fixed unprivileged uid for ssl key access",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.yml"))

				nginxService, ok := compose.Services["nginx"]
				require.True(t, ok, "nginx service missing from compose")
				require.Empty(t, nginxService.User)
			},
		},
		{
			name: "setup e2e playwright bootstrap follows pnpm lockfile contract",
			check: func(t *testing.T, root string) {
				scriptData, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)
				script := string(scriptData)

				readmeData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "README.md"))
				require.NoError(t, err)
				readme := string(readmeData)

				installData, err := os.ReadFile(filepath.Join(root, "docs", "installation.md"))
				require.NoError(t, err)
				installation := string(installData)

				require.FileExists(t, filepath.Join(root, "tests", "e2e", "pnpm-lock.yaml"))
				require.Contains(t, script, "corepack enable")
				require.Contains(t, script, "pnpm install --frozen-lockfile")
				require.Contains(t, script, "pnpm exec playwright install --with-deps")
				require.Contains(t, script, "Run tests with: cd tests/e2e && pnpm test")
				require.NotContains(t, script, "npm ci")
				require.NotContains(t, script, "Run tests with: cd tests/e2e && npm test")
				require.Contains(t, readme, "cd tests/e2e && pnpm test")
				require.NotContains(t, readme, "cd tests/e2e && npm test")
				require.Contains(t, installation, "pnpm install --frozen-lockfile")
				require.Contains(t, installation, "pnpm exec playwright install --with-deps chromium")
				require.Contains(t, installation, "pnpm test")
				require.NotContains(t, installation, "cd tests/e2e\nnpm ci")
				require.NotContains(t, installation, "cd tests/e2e\nnpm test")
				require.Contains(t, installation, "ENCRYPTION_KEY=$(openssl rand -hex 32)")
				require.NotContains(t, installation, "ENCRYPTION_KEY=$(openssl rand -base64 32)")
				require.NotContains(t, installation, "ENCRYPTION_KEY=development_encryption_key_32b")
			},
		},
		{
			name: "installation env examples satisfy validation contract",
			check: func(t *testing.T, root string) {
				installData, err := os.ReadFile(filepath.Join(root, "docs", "installation.md"))
				require.NoError(t, err)
				installation := string(installData)

				require.Regexp(t,
					regexp.MustCompile(`(?s)cat > \.env << EOF.*?DATABASE_URL=postgresql://.*?NATS_URL=nats://.*?ENCRYPTION_KEY=.*?GUEST_OP_HMAC_SECRET=.*?EOF`),
					installation,
				)
				require.Regexp(t,
					regexp.MustCompile(`(?s)set -a\s*\n\. \.env\s*\nset \+a\s*\n\./scripts/validate-env\.sh`),
					installation,
				)
				require.Regexp(t,
					regexp.MustCompile(`(?s)# Create development \.env.*?ENCRYPTION_KEY=[0-9a-f]{64}.*?GUEST_OP_HMAC_SECRET=.*?EOF`),
					installation,
				)
				require.Regexp(t,
					regexp.MustCompile(`(?s)DATABASE_URL="postgresql://.*?NATS_URL="nats://.*?ENCRYPTION_KEY="[0-9a-f]{64}".*?GUEST_OP_HMAC_SECRET="[^"]{32,}".*?\./bin/controller`),
					installation,
				)
			},
		},
		{
			name: "playwright browser selection uses real project names",
			check: func(t *testing.T, root string) {
				packageData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "package.json"))
				require.NoError(t, err)
				pkg := string(packageData)

				configData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "playwright.config.ts"))
				require.NoError(t, err)
				config := string(configData)

				workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e.yml"))
				require.NoError(t, err)
				workflow := string(workflowData)

				readmeData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "README.md"))
				require.NoError(t, err)
				readme := string(readmeData)

				require.Contains(t, config, "name: 'admin-chromium'")
				require.Contains(t, config, "name: 'customer-chromium'")
				require.Contains(t, config, "name: 'auth-chromium'")
				require.NotContains(t, config, "name: 'chromium'")

				require.Contains(t, pkg, `"test:chromium": "playwright test --project=admin-chromium --project=customer-chromium --project=auth-chromium"`)
				require.Contains(t, pkg, `"test:firefox": "playwright test --project=admin-firefox --project=customer-firefox --project=auth-firefox"`)
				require.Contains(t, pkg, `"test:webkit": "playwright test --project=admin-webkit --project=customer-webkit --project=auth-webkit"`)
				require.NotContains(t, pkg, `--project=chromium"`)
				require.NotContains(t, pkg, `--project=firefox"`)
				require.NotContains(t, pkg, `--project=webkit"`)

				require.Contains(t, workflow, "project_group:")
				require.Contains(t, workflow, "default: 'chromium'")
				require.Contains(t, workflow, "pnpm exec playwright install --with-deps ${{ inputs.project_group || 'chromium' }}")
				require.Contains(t, workflow, "pnpm exec playwright test --project=admin-${{ inputs.project_group || 'chromium' }} --project=customer-${{ inputs.project_group || 'chromium' }} --project=auth-${{ inputs.project_group || 'chromium' }}")
				require.NotContains(t, workflow, "--project=${{ inputs.browsers || 'chromium' }}")
				require.NotContains(t, workflow, `gh workflow run e2e.yml -f browsers="chromium,firefox"`)
				require.Contains(t, readme, "pnpm run test:chromium")
				require.Contains(t, readme, "pnpm run test:firefox")
				require.Contains(t, readme, "pnpm run test:webkit")
				require.Contains(t, readme, "pnpm run test:debug")
				require.Contains(t, readme, "pnpm exec playwright show-trace test-results/trace.zip")
				require.Contains(t, readme, `gh workflow run e2e.yml -f project_group="chromium"`)
				require.NotContains(t, readme, "\nnpm run test:chromium")
				require.NotContains(t, readme, "\nnpm run test:debug")
				require.NotContains(t, readme, `gh workflow run e2e.yml -f browsers="chromium,firefox"`)
			},
		},
		{
			name: "playwright reuses prestarted webui servers in ci",
			check: func(t *testing.T, root string) {
				configData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "playwright.config.ts"))
				require.NoError(t, err)
				config := string(configData)

				workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e.yml"))
				require.NoError(t, err)
				workflow := string(workflowData)

				require.Contains(t, workflow, "Start Admin WebUI")
				require.Contains(t, workflow, "PORT=3000 npm run start &")
				require.Contains(t, workflow, "Start Customer WebUI")
				require.Contains(t, workflow, "PORT=3001 npm run start &")
				require.Contains(t, config, "command: 'npm run dev --prefix ../../webui/admin'")
				require.Contains(t, config, "command: 'npm run dev --prefix ../../webui/customer'")
				require.Contains(t, config, "reuseExistingServer: !!process.env.CI")
				require.NotContains(t, config, "reuseExistingServer: !process.env.CI")
			},
		},
		{
			name: "playwright auth state generation is wired to seeded credentials",
			check: func(t *testing.T, root string) {
				configData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "playwright.config.ts"))
				require.NoError(t, err)
				config := string(configData)

				authSetupData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "auth.setup.ts"))
				require.NoError(t, err)
				authSetup := string(authSetupData)

				authUtilsData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "utils", "auth.ts"))
				require.NoError(t, err)
				authUtils := string(authUtilsData)

				readmeData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "README.md"))
				require.NoError(t, err)
				readme := string(readmeData)

				require.Contains(t, config, "name: 'setup-admin'")
				require.Contains(t, config, "testMatch: /auth\\.setup\\.ts/")
				require.Contains(t, config, "name: 'setup-customer'")
				require.Contains(t, config, "dependencies: ['setup-admin']")
				require.Contains(t, config, "dependencies: ['setup-customer']")

				require.Contains(t, authSetup, "Run: pnpm exec playwright test --project=setup-admin --project=setup-customer")

				require.Contains(t, authUtils, "admin@test.virtuestack.local")
				require.Contains(t, authUtils, "2fa-admin@test.virtuestack.local")
				require.Contains(t, authUtils, "customer@test.virtuestack.local")
				require.Contains(t, authUtils, "2fa-customer@test.virtuestack.local")
				require.NotContains(t, authUtils, "admin@virtuestack.local")
				require.NotContains(t, authUtils, "customer@virtuestack.local")

				require.Contains(t, readme, "pnpm exec playwright test --project=setup-admin --project=setup-customer")
				require.NotContains(t, readme, "pnpm exec playwright test auth.setup.ts")
			},
		},
		{
			name: "playwright mobile customer projects depend on seeded auth state",
			check: func(t *testing.T, root string) {
				configData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "playwright.config.ts"))
				require.NoError(t, err)
				config := string(configData)

				require.Regexp(t,
					regexp.MustCompile(`(?s)name: 'mobile-chrome'.*?dependencies: \['setup-customer'\].*?storageState: '\.auth/customer-storage\.json'`),
					config,
				)
				require.Regexp(t,
					regexp.MustCompile(`(?s)name: 'mobile-safari'.*?dependencies: \['setup-customer'\].*?storageState: '\.auth/customer-storage\.json'`),
					config,
				)
			},
		},
		{
			name: "workflow installs the selected playwright browser family",
			check: func(t *testing.T, root string) {
				workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e.yml"))
				require.NoError(t, err)
				workflow := string(workflowData)

				require.Contains(t, workflow, "project_group:")
				require.Contains(t, workflow, "pnpm exec playwright install --with-deps ${{ inputs.project_group || 'chromium' }}")
				require.NotContains(t, workflow, "pnpm exec playwright install --with-deps chromium")
			},
		},
		{
			name: "e2e workflow uses valid controller secrets and fails loud on seed drift",
			check: func(t *testing.T, root string) {
				workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e.yml"))
				require.NoError(t, err)
				workflow := string(workflowData)

				require.Contains(t, workflow, "cat > .env.test << EOF")
				require.Contains(t, workflow, "GUEST_OP_HMAC_SECRET:")
				require.Contains(t, workflow, "GUEST_OP_HMAC_SECRET=")
				require.Contains(t, workflow, "timed out waiting for")
				require.Regexp(t,
					regexp.MustCompile(`(?m)^  ENCRYPTION_KEY:\s*['"]?[0-9a-f]{64}['"]?$`),
					workflow,
				)
				require.NotContains(t, workflow, "http://localhost:8222/healthz")
				require.NotContains(t, workflow, "./scripts/setup-e2e.sh --seed-only || true")
				require.NotContains(t, workflow, "< migrations/test_seed.sql || true")
				require.NotContains(t, workflow, "continue-on-error: true")
			},
		},
		{
			name: "e2e workflow seeds test credentials before seed-only",
			check: func(t *testing.T, root string) {
				workflowData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "e2e.yml"))
				require.NoError(t, err)
				workflow := string(workflowData)

				require.Contains(t, workflow, "TEST_ADMIN_PASSWORD=AdminTest123!")
				require.Contains(t, workflow, "TEST_ADMIN_TOTP_SECRET=JBSWY3DPEHPK3PXP")
				require.Contains(t, workflow, "TEST_CUSTOMER_PASSWORD=CustomerTest123!")
				require.Contains(t, workflow, "TEST_CUSTOMER_TOTP_SECRET=KRSXG5DSN5XW4ZLP")
				require.Regexp(t, regexp.MustCompile(`(?s)cat > \.env\.test << EOF.*TEST_ADMIN_PASSWORD=AdminTest123!.*TEST_CUSTOMER_TOTP_SECRET=KRSXG5DSN5XW4ZLP.*\./scripts/setup-e2e\.sh --seed-only`), workflow)
			},
		},
		{
			name: "e2e auth specs follow seeded account and settings contracts",
			check: func(t *testing.T, root string) {
				authSpecData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "auth.spec.ts"))
				require.NoError(t, err)
				authSpec := string(authSpecData)

				adminFailureData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "admin-auth-profile-failure.spec.ts"))
				require.NoError(t, err)
				adminFailure := string(adminFailureData)

				settingsSpecData, err := os.ReadFile(filepath.Join(root, "tests", "e2e", "customer-settings.spec.ts"))
				require.NoError(t, err)
				settingsSpec := string(settingsSpecData)

				securityTabData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "components", "settings", "SecurityTab.tsx"))
				require.NoError(t, err)
				securityTab := string(securityTabData)

				require.Contains(t, authSpec, "TEST_ADMIN_TOTP_SECRET")
				require.Contains(t, authSpec, "TEST_CUSTOMER_TOTP_SECRET")
				require.NotContains(t, authSpec, "process.env.ADMIN_TOTP_SECRET")
				require.NotContains(t, authSpec, "process.env.CUSTOMER_TOTP_SECRET")
				require.NotContains(t, authSpec, "@virtuestack.local")
				require.NotContains(t, authSpec, "Password123!")
				require.Contains(t, adminFailure, "admin@test.virtuestack.local")
				require.NotContains(t, adminFailure, "admin@virtuestack.local")
				require.Contains(t, authSpec, "baseURL: customerBaseURL")

				require.Contains(t, settingsSpec, "await this.page.goto('/settings');")
				require.NotContains(t, settingsSpec, "await this.page.goto('/settings/security/2fa');")
				require.Contains(t, settingsSpec, "disable-2fa-password")
				require.NotContains(t, settingsSpec, "input[name=\"totp_code\"]', code")
				require.Contains(t, settingsSpec, "TEST_CUSTOMER_TOTP_SECRET")
				require.NotContains(t, settingsSpec, "process.env.CUSTOMER_TOTP_SECRET")

				require.Contains(t, securityTab, "id=\"totp-code\"")
				require.Contains(t, securityTab, "id=\"disable-2fa-password\"")
				require.Contains(t, securityTab, "Enable 2FA above to see the QR code setup")
				require.Contains(t, securityTab, "2FA is enabled")
			},
		},
		{
			name: "setup e2e seed generation follows current auth and storage schema",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)
				script := string(data)

				require.Contains(t, script, "load_env_file()")
				require.Contains(t, script, "INSERT INTO storage_backends")
				require.Contains(t, script, "storage_backend_id")
				require.Contains(t, script, "INSERT INTO webhooks")
				require.Contains(t, script, "INSERT INTO backups (id, vm_id, method")
				require.Contains(t, script, "totp_enabled")
				require.Contains(t, script, "go run")
				require.Contains(t, script, "'assigned'")
				require.NotContains(t, script, "testadminhash1234567890")
				require.NotContains(t, script, "testcusthash1234567890")
				require.NotContains(t, script, "INSERT INTO admins (id, email, password_hash, role, status")
				require.NotContains(t, script, "INSERT INTO customer_webhooks")
				require.NotContains(t, script, "INSERT INTO backups (id, vm_id, type")
				require.NotContains(t, script, "status = 'used'")
			},
		},
		{
			name: "setup e2e seed only is not hardwired to compose postgres",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)
				script := string(data)

				require.Regexp(t, regexp.MustCompile(`(?s)--seed-only\).*?create_seed_sql.*?\bpsql\b`), script)
				require.NotRegexp(t, regexp.MustCompile(`(?s)--seed-only\).*?exec -T postgres`), script)
			},
		},
		{
			name: "swagger refresh endpoints do not require request bodies",
			check: func(t *testing.T, root string) {
				swagger := loadSwaggerFile(t, filepath.Join(root, "docs", "swagger.yaml"))

				assertNoBodyParameter(t, swagger, "/api/v1/admin/auth/refresh", "post")
				assertNoBodyParameter(t, swagger, "/api/v1/customer/auth/refresh", "post")
			},
		},
		{
			name: "compose controller env wires required guest op secret",
			check: func(t *testing.T, root string) {
				composePaths := []string{
					filepath.Join(root, "docker-compose.yml"),
					filepath.Join(root, "docker-compose.prod.yml"),
					filepath.Join(root, "docker-compose.test.yml"),
				}

				for _, composePath := range composePaths {
					compose := loadComposeFile(t, composePath)
					controllerService, ok := compose.Services["controller"]
					require.True(t, ok, "controller service missing from %s", filepath.Base(composePath))

					controllerEnv := composeEnvironmentMap(t, controllerService.Environment)
					require.NotEmpty(t, controllerEnv["GUEST_OP_HMAC_SECRET"], "controller must set GUEST_OP_HMAC_SECRET in %s", filepath.Base(composePath))
				}
			},
		},
		{
			name: "nginx runtime paths stay writable with default master worker model",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.yml"))
				nginxService, ok := compose.Services["nginx"]
				require.True(t, ok, "nginx service missing from docker-compose.yml")

				require.Empty(t, nginxService.User)

				nginxConfData, err := os.ReadFile(filepath.Join(root, "nginx", "nginx.conf"))
				require.NoError(t, err)
				nginxConf := string(nginxConfData)
				require.Contains(t, nginxConf, "user nginx;")
				require.Contains(t, nginxConf, "pid /tmp/nginx.pid;")
				require.Contains(t, nginxConf, "error_log /dev/stderr warn;")
				require.Contains(t, nginxConf, "access_log /dev/stdout main;")
				require.Contains(t, nginxConf, "client_body_temp_path /tmp/nginx-client-body;")
				require.Contains(t, nginxConf, "proxy_temp_path /tmp/nginx-proxy;")
				require.Contains(t, nginxConf, "fastcgi_temp_path /tmp/nginx-fastcgi;")
				require.Contains(t, nginxConf, "uwsgi_temp_path /tmp/nginx-uwsgi;")
				require.Contains(t, nginxConf, "scgi_temp_path /tmp/nginx-scgi;")
				require.NotContains(t, nginxConf, "/var/run/nginx.pid")
				require.NotContains(t, nginxConf, "/var/log/nginx/")

				defaultConfData, err := os.ReadFile(filepath.Join(root, "nginx", "conf.d", "default.conf"))
				require.NoError(t, err)
				defaultConf := string(defaultConfData)
				require.Contains(t, defaultConf, "access_log /dev/stdout json_combined;")
				require.Contains(t, defaultConf, "error_log /dev/stderr warn;")
				require.NotContains(t, defaultConf, "/var/log/nginx/")
			},
		},
		{
			name: "setup e2e generates guest op secret",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				script := string(data)
				require.Contains(t, script, "GUEST_OP_HMAC_SECRET=")
				require.Regexp(t, regexp.MustCompile(`GUEST_OP_HMAC_SECRET=\$\((generate_random_string 48|openssl rand -base64 48)\)`), script)
			},
		},
		{
			name: "setup e2e uses url safe db and nats secrets",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)
				script := string(data)

				require.Regexp(t, regexp.MustCompile(`POSTGRES_PASSWORD=\$\((generate_random_hex \d+|openssl rand -hex \d+)\)`), script)
				require.Regexp(t, regexp.MustCompile(`NATS_AUTH_TOKEN=\$\((generate_random_hex \d+|openssl rand -hex \d+)\)`), script)
				require.NotContains(t, script, "POSTGRES_PASSWORD=$(generate_random_string")
				require.NotContains(t, script, "NATS_AUTH_TOKEN=$(generate_random_string")
			},
		},
		{
			name: "backup config creates private temp workspace",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "backup-config.sh"))
				require.NoError(t, err)

				require.Contains(t, string(data), "mktemp -d")
			},
		},
		{
			name: "backup config restricts output file permissions",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "backup-config.sh"))
				require.NoError(t, err)

				require.Contains(t, string(data), `chmod 600 "${output_file}"`)
			},
		},
		{
			name: "customer serial console no longer fetches a query token",
			check: func(t *testing.T, root string) {
				pageData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "app", "vms", "[id]", "page.tsx"))
				require.NoError(t, err)

				source := string(pageData)
				require.NotContains(t, source, "getSerialToken(")
				require.NotContains(t, source, "SerialConsoleWithToken")
			},
		},
		{
			name: "customer serial console websocket url omits token query",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "webui", "customer", "components", "serial-console", "serial-console.tsx"))
				require.NoError(t, err)

				source := string(data)
				require.NotContains(t, source, "?token=")
				require.NotContains(t, source, "tokenParam")
			},
		},
		{
			name: "admin require auth suppresses redirect during bootstrap failure",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "webui", "admin", "lib", "require-auth.tsx"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, "hasBootstrapError")
				require.Contains(t, source, "getProtectedRouteView")
			},
		},
		{
			name: "bundled monitoring config uses reachable metrics listeners",
			check: func(t *testing.T, root string) {
				compose := loadComposeFile(t, filepath.Join(root, "docker-compose.yml"))
				controllerService, ok := compose.Services["controller"]
				require.True(t, ok, "controller service missing from docker-compose.yml")

				controllerEnv := composeEnvironmentMap(t, controllerService.Environment)
				require.Equal(t, "${CONTROLLER_METRICS_ADDR:-:9091}", controllerEnv["METRICS_ADDR"])

				prometheusData, err := os.ReadFile(filepath.Join(root, "configs", "prometheus", "prometheus.yml"))
				require.NoError(t, err)
				prometheusSource := string(prometheusData)
				require.Contains(t, prometheusSource, "targets: ['controller:9091']")
				require.NotContains(t, prometheusSource, "targets: ['controller:8080']")

				nodeAgentData, err := os.ReadFile(filepath.Join(root, "configs", "nodeagent.yaml"))
				require.NoError(t, err)
				require.Contains(t, string(nodeAgentData), `metrics_addr: "0.0.0.0:9091"`)
			},
		},
		{
			name: "admin ui stays base path aware under nginx admin mount",
			check: func(t *testing.T, root string) {
				nextConfigData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "next.config.js"))
				require.NoError(t, err)
				nextConfigSource := string(nextConfigData)
				require.Contains(t, nextConfigSource, "basePath: '/admin'")

				pathnameData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "lib", "pathname.ts"))
				require.NoError(t, err)
				pathnameSource := string(pathnameData)
				require.Contains(t, pathnameSource, `export const ADMIN_BASE_PATH = "/admin"`)
				require.Contains(t, pathnameSource, "stripAdminBasePath")
				require.Contains(t, pathnameSource, "isAdminLoginPath")
				require.Contains(t, pathnameSource, "isAdminNavItemActive")

				adminShellData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "components", "admin-shell.tsx"))
				require.NoError(t, err)
				adminShellSource := string(adminShellData)
				require.Contains(t, adminShellSource, "isAdminLoginPath(pathname)")
				require.NotContains(t, adminShellSource, `pathname === "/login"`)

				sidebarData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "components", "sidebar.tsx"))
				require.NoError(t, err)
				sidebarSource := string(sidebarData)
				require.Contains(t, sidebarSource, "isAdminNavItemActive(pathname, item.href)")
				require.NotContains(t, sidebarSource, `pathname === item.href`)
				require.NotContains(t, sidebarSource, `src="/avatars/admin.png"`)

				mobileNavData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "components", "mobile-nav.tsx"))
				require.NoError(t, err)
				mobileNavSource := string(mobileNavData)
				require.Contains(t, mobileNavSource, "isAdminNavItemActive(pathname, item.href)")
				require.NotContains(t, mobileNavSource, `pathname === item.href`)
			},
		},
		{
			name: "nginx admin and customer proxy blocks do not nest static locations",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "nginx", "conf.d", "default.conf"))
				require.NoError(t, err)

				source := string(data)
				adminBlock := directiveBlock(t, source, "location /admin {")
				require.NotContains(t, adminBlock, "location ~*")

				customerBlock := directiveBlock(t, source, "location / {\n        limit_req zone=customer_limit burst=50 nodelay;")
				require.NotContains(t, customerBlock, "location ~*")
			},
		},
		{
			name: "nginx next static locations outrank regex asset caching",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "nginx", "conf.d", "default.conf"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, "location ^~ /admin/_next/static/")
				require.Contains(t, source, "location ^~ /_next/static/")
			},
		},
		{
			name: "customer vnc console no longer depends on tokenized websocket urls",
			check: func(t *testing.T, root string) {
				tabData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "components", "vm", "VMConsoleTab.tsx"))
				require.NoError(t, err)

				tabSource := string(tabData)
				require.NotContains(t, tabSource, "getConsoleToken")
				require.NotContains(t, tabSource, "response.url")
				require.NotContains(t, tabSource, "wsUrl=")
				require.Contains(t, tabSource, "<VNCConsole")
				require.Contains(t, tabSource, "vmId={vmId}")
			},
		},
		{
			name: "customer vnc websocket url omits token query parameter",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "webui", "customer", "components", "novnc-console", "vnc-console.tsx"))
				require.NoError(t, err)

				source := string(data)
				require.NotContains(t, source, "?token=")
				require.NotContains(t, source, "wsUrl?: string")
				require.NotContains(t, source, "token?: string")
				require.Contains(t, source, "/api/v1/customer/ws/vnc/${vmId}")
			},
		},
		{
			name: "nginx upgrades customer websocket console routes under api prefix",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "nginx", "conf.d", "default.conf"))
				require.NoError(t, err)

				source := string(data)
				wsLocation := `location ^~ /api/v1/customer/ws/ {`
				apiLocation := `location /api/v1/ {`
				require.Contains(t, source, wsLocation)
				require.Contains(t, source, `proxy_pass http://controller/api/v1/customer/ws/;`)
				require.Contains(t, source, `proxy_set_header Upgrade $http_upgrade;`)
				require.Contains(t, source, `proxy_set_header Connection "upgrade";`)
				require.Contains(t, source, `proxy_buffering off;`)
				require.Less(t, strings.Index(source, wsLocation), strings.Index(source, apiLocation), "websocket location must outrank generic api proxy")
			},
		},
		{
			name: "customer api client drops unused vnc token helper",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "webui", "customer", "lib", "api-client.ts"))
				require.NoError(t, err)

				source := string(data)
				require.NotContains(t, source, "interface ConsoleTokenResponse")
				require.NotContains(t, source, "getConsoleToken")
			},
		},
		{
			name: "ensure template cache keys by template id not sanitized name",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "server.go"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, `template_id`)
				require.NotContains(t, source, `ref := storage.SanitizeTemplateName(req.TemplateName)`)
				require.Contains(t, source, `req.TemplateId`)
			},
		},
		{
			name: "lvm cache path uses canonical dev path contract",
			check: func(t *testing.T, root string) {
				serverData, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "template_cache_paths.go"))
				require.NoError(t, err)

				serverSource := string(serverData)
				require.Contains(t, serverSource, `return canonicalLVMTemplatePath(lvmVG, ref)`)
				require.Contains(t, serverSource, `return fmt.Sprintf("/dev/%s/%s", vgName, lvName), nil`)

				lvmData, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "storage", "lvm_template.go"))
				require.NoError(t, err)

				lvmSource := string(lvmData)
				require.Contains(t, lvmSource, `return lvPath, virtualSize, nil`)
				require.Contains(t, lvmSource, `NormalizeLVMTemplateRef`)
			},
		},
		{
			name: "template downloads do not reuse iso ssrf dialer",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "storage", "template_builder.go"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, `func (b *TemplateBuilder) DownloadFile`)
				require.NotContains(t, source, `client := downloadutil.NewHTTPClient(ssrfSafeDialContext(), maxRedirects)`)
				require.Contains(t, source, `downloadutil.NewHTTPClient(`)
				require.Contains(t, source, `ssrfSafeDialContext()`)
			},
		},
		{
			name: "qcow template import publishes atomically",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "storage", "qcow_template.go"))
				require.NoError(t, err)

				source := string(data)
				require.NotContains(t, source, `m.copyFile(sourcePath, targetPath)`)
				require.NotContains(t, source, `os.Create(dst)`)
				require.Contains(t, source, `os.Rename(`)
				require.Contains(t, source, `O_CREATE|os.O_WRONLY|os.O_EXCL`)
			},
		},
		{
			name: "customer webhook encrypted secret storage is not capped at varchar 128",
			check: func(t *testing.T, root string) {
				migration, err := os.ReadFile(filepath.Join(root, "migrations", "000081_widen_webhook_secret_storage.up.sql"))
				require.NoError(t, err)

				source := string(migration)
				require.Contains(t, source, `ALTER TABLE webhooks`)
				require.Contains(t, source, `ALTER COLUMN secret_hash TYPE TEXT`)
				require.Contains(t, source, `Encrypted customer webhook secret`)
			},
		},
		{
			name: "system webhook deliveries use durable idempotent storage",
			check: func(t *testing.T, root string) {
				migration, err := os.ReadFile(filepath.Join(root, "migrations", "000082_system_webhook_deliveries.up.sql"))
				require.NoError(t, err)

				source := string(migration)
				require.Contains(t, source, `CREATE TABLE IF NOT EXISTS system_webhook_deliveries`)
				require.Contains(t, source, `idempotency_key TEXT NOT NULL`)
				require.Contains(t, source, `UNIQUE (idempotency_key)`)
				require.Contains(t, source, `idx_system_webhook_deliveries_pending`)
				require.Contains(t, source, `update_webhook_updated_at()`)
			},
		},
		{
			name: "node agent guest op verification fails closed when secret is missing",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "grpc_handlers_guest.go"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, `"fmt"`)
				require.Contains(t, source, `"strings"`)
				require.Contains(t, source, `if secret == "" {`)
				require.Contains(t, source, `return status.Error(codes.FailedPrecondition, "guest operation HMAC secret is not configured")`)
				require.Contains(t, source, "sharedcrypto.VerifyGuestOpToken")
				require.Contains(t, source, "codes.FailedPrecondition")
				require.Contains(t, source, "guest operation HMAC secret is not configured")
				require.NotContains(t, source, "skipping per-operation token verification")
			},
		},
		{
			name: "snapshot revert honors stopped precondition",
			check: func(t *testing.T, root string) {
				handlerData, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "grpc_handlers_snapshot.go"))
				require.NoError(t, err)
				policyData, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "snapshotpolicy", "revert.go"))
				require.NoError(t, err)

				handlerSource := string(handlerData)
				policySource := string(policyData)
				require.Contains(t, handlerSource, `snapshotpolicy.RevertSnapshot(`)
				require.NotContains(t, handlerSource, `ForceStopVM(ctx, req.GetVmId())`)
				require.Contains(t, policySource, `grpcstatus.Error(codes.FailedPrecondition, "VM must be stopped before reverting snapshot")`)
				require.Contains(t, policySource, `AllowsRevert(vmStatus)`)
			},
		},
		{
			name: "auth revalidation forces tab-return checks and guards stale responses",
			check: func(t *testing.T, root string) {
				adminContextData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "lib", "auth-context.tsx"))
				require.NoError(t, err)
				adminBootstrapData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "lib", "auth-bootstrap.ts"))
				require.NoError(t, err)
				customerContextData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "lib", "auth-context.tsx"))
				require.NoError(t, err)
				customerBootstrapData, err := os.ReadFile(filepath.Join(root, "webui", "customer", "lib", "auth-bootstrap.ts"))
				require.NoError(t, err)

				adminContext := string(adminContextData)
				adminBootstrap := string(adminBootstrapData)
				customerContext := string(customerContextData)
				customerBootstrap := string(customerBootstrapData)

				require.Contains(t, adminContext, `void revalidateSession({ force: true })`)
				require.Contains(t, customerContext, `void revalidateSession({ force: true })`)
				require.Contains(t, adminContext, `applyRevalidationResultIfCurrent(`)
				require.Contains(t, customerContext, `applyRevalidationResultIfCurrent(`)
				require.Contains(t, adminBootstrap, `force?: boolean`)
				require.Contains(t, customerBootstrap, `force?: boolean`)
				require.Contains(t, adminBootstrap, `expectedRequestId !== latestRequestId`)
				require.Contains(t, customerBootstrap, `expectedRequestId !== latestRequestId`)
				require.Contains(t, adminContext, `revalidationInFlightRef.current`)
				require.Contains(t, customerContext, `revalidationInFlightRef.current`)
			},
		},
		{
			name: "console streams enforce guest op token verification",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "grpc_handlers_console.go"))
				require.NoError(t, err)

				source := string(data)
				vncStart := strings.Index(source, `func (h *grpcHandler) StreamVNCConsole(`)
				serialStart := strings.Index(source, `func (h *grpcHandler) StreamSerialConsole(`)
				require.NotEqual(t, -1, vncStart)
				require.NotEqual(t, -1, serialStart)
				require.Greater(t, serialStart, vncStart)

				vncSource := source[vncStart:serialStart]
				serialSource := source[serialStart:]

				require.Contains(t, vncSource, `if err := h.verifyGuestOpToken(stream.Context(), vmID); err != nil {`)
				require.Contains(t, serialSource, `if err := h.verifyGuestOpToken(stream.Context(), vmID); err != nil {`)

				vncVerifyIndex := strings.Index(vncSource, `if err := h.verifyGuestOpToken(stream.Context(), vmID); err != nil {`)
				vncLookupIndex := strings.Index(vncSource, `LookupDomainByName(vm.DomainNameFromID(vmID))`)
				require.NotEqual(t, -1, vncVerifyIndex)
				require.NotEqual(t, -1, vncLookupIndex)
				require.Less(t, vncVerifyIndex, vncLookupIndex)

				serialVerifyIndex := strings.Index(serialSource, `if err := h.verifyGuestOpToken(stream.Context(), vmID); err != nil {`)
				serialLookupIndex := strings.Index(serialSource, `LookupDomainByName(vm.DomainNameFromID(vmID))`)
				require.NotEqual(t, -1, serialVerifyIndex)
				require.NotEqual(t, -1, serialLookupIndex)
				require.Less(t, serialVerifyIndex, serialLookupIndex)
			},
		},
		{
			name: "node agent metrics fallback binds loopback",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "internal", "nodeagent", "server.go"))
				require.NoError(t, err)

				source := string(data)
				require.Contains(t, source, `metricsAddr = "127.0.0.1:9091"`)
				require.NotContains(t, source, `metricsAddr = ":9091"`)
			},
		},
		{
			name: "admin backups route exists for navigation target",
			check: func(t *testing.T, root string) {
				_, err := os.Stat(filepath.Join(root, "webui", "admin", "app", "backups", "page.tsx"))
				require.NoError(t, err)
			},
		},
		{
			name: "admin customer detail route exists for view action",
			check: func(t *testing.T, root string) {
				_, err := os.Stat(filepath.Join(root, "webui", "admin", "app", "customers", "[id]", "page.tsx"))
				require.NoError(t, err)
			},
		},
		{
			name: "admin customer vm subroute exists",
			check: func(t *testing.T, root string) {
				_, err := os.Stat(filepath.Join(root, "webui", "admin", "app", "customers", "[id]", "vms", "page.tsx"))
				require.NoError(t, err)
			},
		},
		{
			name: "admin customer audit logs subroute exists",
			check: func(t *testing.T, root string) {
				_, err := os.Stat(filepath.Join(root, "webui", "admin", "app", "customers", "[id]", "audit-logs", "page.tsx"))
				require.NoError(t, err)
			},
		},
		{
			name: "admin root layout owns shared authenticated shell",
			check: func(t *testing.T, root string) {
				layoutData, err := os.ReadFile(filepath.Join(root, "webui", "admin", "app", "layout.tsx"))
				require.NoError(t, err)
				require.Contains(t, string(layoutData), "AdminShell")
			},
		},
		{
			name: "dashboard route no longer owns admin shell alone",
			check: func(t *testing.T, root string) {
				layoutPath := filepath.Join(root, "webui", "admin", "app", "dashboard", "layout.tsx")
				layoutData, err := os.ReadFile(layoutPath)
				if os.IsNotExist(err) {
					return
				}
				require.NoError(t, err)
				require.NotContains(t, string(layoutData), "RequireAuth")
				require.NotContains(t, string(layoutData), "Sidebar")
				require.NotContains(t, string(layoutData), "MobileNav")
			},
		},
		{
			name: "whmcs welcome emails do not include vm passwords",
			check: func(t *testing.T, root string) {
				webhookData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "webhook.php"))
				require.NoError(t, err)
				require.NotContains(t, string(webhookData), "'password' => $vmData['password']")
				require.NotContains(t, string(webhookData), "'password' => $password")

				hooksData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "hooks.php"))
				require.NoError(t, err)
				require.NotContains(t, string(hooksData), "'password' => $vmData['password']")
				require.NotContains(t, string(hooksData), "'password' => $result['password']")
			},
		},
		{
			name: "whmcs client area allows secure power actions and password guidance",
			check: func(t *testing.T, root string) {
				moduleData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "virtuestack.php"))
				require.NoError(t, err)
				require.Contains(t, string(moduleData), "function virtuestack_ClientAreaAllowedFunctions(")
				require.Contains(t, string(moduleData), "'startVM'")
				require.Contains(t, string(moduleData), "'stopVM'")
				require.Contains(t, string(moduleData), "'restartVM'")
				require.Contains(t, string(moduleData), "function virtuestack_ChangePassword(")

				templateData, err := os.ReadFile(filepath.Join(root, "modules", "servers", "virtuestack", "templates", "overview.tpl"))
				require.NoError(t, err)
				require.Contains(t, string(templateData), "root passwords are not sent by email")
				require.Contains(t, string(templateData), "Change Password")
				require.NotContains(t, string(templateData), "modop=custom&a=openConsole")
			},
		},
	}

	root := repoRoot(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, root)
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func loadComposeFile(t *testing.T, path string) composeFile {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var compose composeFile
	require.NoError(t, yaml.Unmarshal(data, &compose))
	return compose
}

func loadSwaggerFile(t *testing.T, path string) swaggerFile {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var swagger swaggerFile
	require.NoError(t, yaml.Unmarshal(data, &swagger))
	return swagger
}

func assertNoBodyParameter(t *testing.T, swagger swaggerFile, path, method string) {
	t.Helper()

	pathItem, ok := swagger.Paths[path]
	require.True(t, ok, "swagger path %s missing", path)

	op, ok := pathItem[method]
	require.True(t, ok, "swagger method %s %s missing", method, path)

	for _, param := range op.Parameters {
		require.NotEqual(t, "body", param.In, "%s %s must not declare a request body parameter", strings.ToUpper(method), path)
	}
}

func directiveBlock(t *testing.T, source, marker string) string {
	t.Helper()

	start := strings.Index(source, marker)
	require.NotEqual(t, -1, start, "directive %q missing", marker)

	openOffset := strings.Index(source[start:], "{")
	require.NotEqual(t, -1, openOffset, "directive %q missing opening brace", marker)

	openIndex := start + openOffset
	depth := 0
	for i := openIndex; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[openIndex+1 : i]
			}
		}
	}

	t.Fatalf("directive %q missing closing brace", marker)
	return ""
}

func composeCommandString(t *testing.T, value any) string {
	t.Helper()

	switch command := value.(type) {
	case string:
		return command
	case []any:
		parts := make([]string, 0, len(command))
		for _, item := range command {
			part, ok := item.(string)
			require.True(t, ok, "compose command list must contain only strings")
			parts = append(parts, part)
		}
		return strings.Join(parts, " ")
	default:
		t.Fatalf("unexpected compose command type %T", value)
		return ""
	}
}

func composeEnvironmentMap(t *testing.T, value any) map[string]string {
	t.Helper()

	switch env := value.(type) {
	case nil:
		return map[string]string{}
	case map[string]any:
		result := make(map[string]string, len(env))
		for key, rawValue := range env {
			strValue, ok := rawValue.(string)
			require.True(t, ok, "compose environment map values must be strings")
			result[key] = strValue
		}
		return result
	case []any:
		result := make(map[string]string, len(env))
		for _, rawItem := range env {
			item, ok := rawItem.(string)
			require.True(t, ok, "compose environment list entries must be strings")
			key, value, ok := strings.Cut(item, "=")
			require.True(t, ok, "compose environment list entries must be KEY=value strings")
			result[key] = value
		}
		return result
	default:
		t.Fatalf("unexpected compose environment type %T", value)
		return nil
	}
}
