package testenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
)

const maxContainerLogBytes = 64 * 1024

var credentialsInURL = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s/@:]+:)[^\s/@]+(@)`)

func (e *Environment) reportFailedContainers(ctx context.Context, out io.Writer) error {
	docker, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return fmt.Errorf("connect to Docker: %w", err)
	}
	defer docker.Close()

	containers, err := docker.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", "com.docker.compose.project="+e.project),
	})
	if err != nil {
		return fmt.Errorf("list test containers: %w", err)
	}

	var inspected []container.InspectResponse
	var secrets []string
	for _, item := range containers.Items {
		info, err := docker.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("inspect test container %s: %w", item.ID, err)
		}
		if info.Container.Config == nil {
			return fmt.Errorf("test container %s has no configuration", item.ID)
		}
		secrets = append(secrets, secretsFromEnv(info.Container.Config.Env)...)
		inspected = append(inspected, info.Container)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })

	var errs []error
	for _, info := range inspected {
		state := info.State
		if state == nil {
			return fmt.Errorf("test container %s has no state", info.ID)
		}
		unhealthy := state.Health != nil && state.Health.Status == container.Unhealthy
		failed := ((state.Status == "exited" || state.Restarting) && state.ExitCode != 0) ||
			state.Dead || state.OOMKilled
		if !failed && !unhealthy {
			continue
		}

		service := info.Config.Labels["com.docker.compose.service"]
		fmt.Fprintf(out, "\n--- E2E container %s (state=%s, exit=%d, unhealthy=%t, oom=%t) ---\n",
			service, state.Status, state.ExitCode, unhealthy, state.OOMKilled)
		logs, err := docker.ContainerLogs(ctx, info.ID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "100",
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("read logs for service %q: %w", service, err))
			continue
		}
		limited := &io.LimitedReader{R: logs, N: maxContainerLogBytes + 1}
		var output bytes.Buffer
		if info.Config.Tty {
			_, err = io.Copy(&output, limited)
		} else {
			_, err = stdcopy.StdCopy(&output, &output, limited)
		}
		closeErr := logs.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("read logs for service %q: %w", service, err))
			continue
		}
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close logs for service %q: %w", service, closeErr))
		}
		text := output.String()
		if limited.N == 0 {
			if end := strings.LastIndexByte(text, '\n'); end >= 0 {
				text = text[:end+1]
			} else {
				text = ""
			}
		}
		fmt.Fprint(out, redactLogs(text, secrets))
		if limited.N == 0 {
			fmt.Fprintln(out, "[log output truncated]")
		}
	}
	return errors.Join(errs...)
}

func secretsFromEnv(env []string) []string {
	var secrets []string
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" {
			continue
		}
		key = strings.ToUpper(key)
		if sensitiveKey(key) || strings.HasSuffix(key, "_DSN") {
			secrets = append(secrets, value)
		}
		if strings.HasSuffix(key, "_DSN") {
			if cfg, err := mysqldriver.ParseDSN(value); err == nil && cfg.Passwd != "" {
				secrets = append(secrets, cfg.Passwd)
			}
		}
		if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			if parsed.User != nil {
				if password, ok := parsed.User.Password(); ok && password != "" {
					secrets = append(secrets, password, url.QueryEscape(password), url.PathEscape(password))
				}
			}
			for name, values := range parsed.Query() {
				if sensitiveKey(strings.ToUpper(name)) {
					for _, secret := range values {
						if secret != "" {
							secrets = append(secrets, secret, url.QueryEscape(secret))
						}
					}
				}
			}
		}
	}
	return secrets
}

func sensitiveKey(key string) bool {
	return strings.Contains(key, "PASSWORD") || strings.Contains(key, "PASSWD") ||
		strings.Contains(key, "SECRET") || strings.Contains(key, "TOKEN") ||
		strings.Contains(key, "COOKIE") || strings.Contains(key, "CREDENTIAL") ||
		key == "PASS" || key == "PWD" || key == "KEY" ||
		strings.HasSuffix(key, "_PASS") || strings.HasSuffix(key, "_PWD") ||
		strings.HasSuffix(key, "_KEY")
}

func redactLogs(logs string, secrets []string) string {
	for _, secret := range secrets {
		logs = strings.ReplaceAll(logs, secret, "[REDACTED]")
	}
	return credentialsInURL.ReplaceAllString(logs, `${1}[REDACTED]${2}`)
}
