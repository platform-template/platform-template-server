package testenv

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
)

func (e *Environment) removeProjectVolumes(ctx context.Context) (result error) {
	docker, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, docker.Close()) }()

	volumes, err := docker.VolumeList(ctx, client.VolumeListOptions{
		Filters: make(client.Filters).Add("label", "com.docker.compose.project="+e.project),
	})
	if err != nil {
		return err
	}
	var errs []error
	for _, volume := range volumes.Items {
		if volume.Labels["com.docker.compose.project"] != e.project ||
			!strings.HasPrefix(volume.Name, e.project+"_") {
			errs = append(errs, fmt.Errorf("unexpected volume %q has the test project label", volume.Name))
			continue
		}
		retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		for {
			_, err = docker.VolumeRemove(retryCtx, volume.Name, client.VolumeRemoveOptions{})
			if err == nil || errdefs.IsNotFound(err) {
				break
			}
			if !errdefs.IsConflict(err) {
				break
			}
			select {
			case <-retryCtx.Done():
				err = errors.Join(err, retryCtx.Err())
			case <-time.After(100 * time.Millisecond):
				continue
			}
			break
		}
		cancel()
		if err != nil && !errdefs.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("remove %q: %w", volume.Name, err))
		}
	}
	return errors.Join(errs...)
}
