//go:build e2e

package testenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestPartialStartupFailureCleansResources(t *testing.T) {
	label := fmt.Sprintf("e2e.lifecycle=%d", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, kind := range []string{"container", "network", "volume"} {
			listArgs := []string{kind, "ls", "-q", "--filter", "label=" + label}
			if kind == "container" {
				listArgs = append(listArgs, "--all")
			}
			output, err := exec.Command("docker", listArgs...).Output()
			if err != nil {
				t.Errorf("list %s for cleanup: %v", kind, err)
				continue
			}
			for _, id := range strings.Fields(string(output)) {
				args := []string{kind, "rm"}
				if kind == "container" {
					args = append(args, "--force")
				}
				args = append(args, id)
				if output, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
					t.Errorf("cleanup %s: %v: %s", kind, err, output)
				}
			}
		}
	})
	config := fmt.Sprintf(`
services:
  started:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "exec sleep 120"]
    labels: ["%[1]s"]
    volumes: ["data:/tmp/e2e-data"]
  fails:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "exit 23"]
    labels: ["%[1]s"]
    depends_on:
      started:
        condition: service_started
volumes:
  data:
    labels: ["%[1]s"]
networks:
  default:
    labels: ["%[1]s"]
`, label)
	path := filepath.Join(t.TempDir(), "compose.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	env, err := Start(ctx, path)
	require.Nil(t, env)
	require.ErrorContains(t, err, "start compose stack")
	require.ErrorContains(t, err, "fails")
	startupErr := err
	for _, args := range [][]string{
		{"ps", "-aq", "--filter", "label=" + label},
		{"network", "ls", "-q", "--filter", "label=" + label},
		{"volume", "ls", "-q", "--filter", "label=" + label},
	} {
		output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		require.NoError(t, err, string(output))
		require.Empty(t, strings.TrimSpace(string(output)), "resources remain: %s; startup error: %v", args, startupErr)
	}
}

func TestCloseReportsErrorsAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	config := `
services:
  unused:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "exec sleep 120"]
`
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	env, err := Start(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if !env.closed {
			_ = env.Close()
		}
	})
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	env.connections["already-closed"] = conn
	err = env.Close()
	require.ErrorContains(t, err, "close already-closed gRPC")
	require.NotContains(t, err.Error(), "remove compose stack")
	require.NotContains(t, err.Error(), "close compose client")
	require.Equal(t, err, env.Close())
	_, err = env.GRPC(t.Context(), "unused")
	require.ErrorContains(t, err, "environment is closed")
}

func TestHTTPReadinessUsesComposeLabel(t *testing.T) {
	config := `
services:
  independent:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "exec sleep 120"]
    labels:
      e2e.http-health-port: ""
`
	path := filepath.Join(t.TempDir(), "compose.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	env, err := Start(ctx, path)
	require.Nil(t, env)
	require.ErrorContains(t, err, `compose service "independent" has an empty HTTP readiness port`)
}

func TestHTTPReadinessWaitsForService(t *testing.T) {
	config := `
services:
  independent:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "exec sleep 120"]
    ports: ["8082"]
    labels:
      e2e.http-health-port: "8082/tcp"
`
	path := filepath.Join(t.TempDir(), "compose.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	env, err := Start(ctx, path)
	require.Nil(t, env)
	require.ErrorContains(t, err, `service "independent" HTTP readiness`)
}

func TestRemoveRemainingProjectVolume(t *testing.T) {
	project := fmt.Sprintf("e2e-volume-test-%d", time.Now().UnixNano())
	volume := project + "_data"
	output, err := exec.Command("docker", "volume", "create",
		"--label", "com.docker.compose.project="+project, volume,
	).CombinedOutput()
	require.NoError(t, err, string(output))
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", volume).Run() })

	env := &Environment{project: project}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, env.removeProjectVolumes(ctx))
	output, err = exec.CommandContext(ctx, "docker", "volume", "ls", "-q",
		"--filter", "label=com.docker.compose.project="+project,
	).CombinedOutput()
	require.NoError(t, err, string(output))
	require.Empty(t, strings.TrimSpace(string(output)))
}

func TestRemoveProjectVolumesRejectsUnexpectedName(t *testing.T) {
	project := fmt.Sprintf("e2e-volume-test-%d", time.Now().UnixNano())
	volume := project + "-external"
	ownVolume := project + "_data"
	output, err := exec.Command("docker", "volume", "create",
		"--label", "com.docker.compose.project="+project, volume,
	).CombinedOutput()
	require.NoError(t, err, string(output))
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", volume, ownVolume).Run() })
	output, err = exec.Command("docker", "volume", "create",
		"--label", "com.docker.compose.project="+project, ownVolume,
	).CombinedOutput()
	require.NoError(t, err, string(output))

	env := &Environment{project: project}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.ErrorContains(t, env.removeProjectVolumes(ctx), "unexpected volume")
	output, err = exec.CommandContext(ctx, "docker", "volume", "inspect", volume).CombinedOutput()
	require.NoError(t, err, string(output))
	output, err = exec.CommandContext(ctx, "docker", "volume", "ls", "-q",
		"--filter", "label=com.docker.compose.project="+project,
	).CombinedOutput()
	require.NoError(t, err, string(output))
	require.Equal(t, volume, strings.TrimSpace(string(output)))
}

func TestReportFailedContainerLogs(t *testing.T) {
	config := `
services:
  healthy:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "echo healthy-only-marker; exec sleep 120"]
    environment:
      OTHER_SECRET: cross-service-secret
  fails:
    image: mysql:8.4
    entrypoint: ["sh", "-c", "echo failure-marker; echo $$AUTH_TOKEN; echo cross-service-secret; echo amqp://user:url-secret@broker; echo failure-stderr >&2; exit 23"]
    environment:
      AUTH_TOKEN: token-secret
      AMQP_URL: amqp://user:url-secret@broker
    depends_on:
      healthy:
        condition: service_started
`
	path := filepath.Join(t.TempDir(), "compose.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	project := fmt.Sprintf("e2e-log-test-%d", time.Now().UnixNano())
	stack, err := compose.NewDockerComposeWith(compose.StackIdentifier(project), compose.WithStackFiles(path))
	require.NoError(t, err)
	env := &Environment{stack: stack, project: project}
	t.Cleanup(func() { require.NoError(t, env.Close()) })

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	require.ErrorContains(t, env.start(ctx), "fails")
	var output bytes.Buffer
	require.NoError(t, env.reportFailedContainers(ctx, &output))
	logs := output.String()
	require.Contains(t, logs, "E2E container fails")
	require.Contains(t, logs, "exit=23")
	require.Contains(t, logs, "failure-marker")
	require.Contains(t, logs, "failure-stderr")
	require.NotContains(t, logs, "healthy-only-marker")
	for _, secret := range []string{"token-secret", "cross-service-secret", "url-secret"} {
		require.NotContains(t, logs, secret)
	}
	require.Contains(t, logs, "[REDACTED]")
}
