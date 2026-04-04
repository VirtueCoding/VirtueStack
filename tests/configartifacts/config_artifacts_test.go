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
	Command     any `yaml:"command"`
	Environment any `yaml:"environment"`
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
			name: "setup e2e generates hex encryption key",
			check: func(t *testing.T, root string) {
				data, err := os.ReadFile(filepath.Join(root, "scripts", "setup-e2e.sh"))
				require.NoError(t, err)

				require.Regexp(t, regexp.MustCompile(`ENCRYPTION_KEY=\$\((generate_random_hex 32|openssl rand -hex 32)\)`), string(data))
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
