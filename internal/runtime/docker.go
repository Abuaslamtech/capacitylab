package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// DockerClient wraps the official Docker Engine client
type DockerClient struct {
	cli *client.Client
}

// ContainerMetrics holds snapshot resource telemetry
type ContainerMetrics struct {
	CPUPercent       float64
	MemoryUsedMB     float64
	MemoryLimitMB    float64
	MemoryPercent    float64
	ThrottledTimeUs  uint64
	ThrottledPeriods uint64
}

// DockerStatsJSON models the relevant fields returned by Docker's stats API
type DockerStatsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
		ThrottlingData struct {
			ThrottledPeriods uint64 `json:"throttled_periods"`
			ThrottledTime    uint64 `json:"throttled_time"`
		} `json:"throttling_data"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			InactiveFile uint64 `json:"inactive_file"`
		} `json:"stats"`
	} `json:"memory_stats"`
}

// NewClient initializes and pings the local Docker daemon
func NewClient() (*DockerClient, error) {
	// client.FromEnv reads standard DOCKER_HOST and unix socket paths
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Ping the daemon with a 3-second timeout to verify it is active
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker daemon: %w (is Docker running? Try: sudo systemctl start docker)", err)
	}

	return &DockerClient{cli: cli}, nil
}

// ContainerStatus verifies if a container exists and whether it is running
func (d *DockerClient) ContainerStatus(ctx context.Context, containerName string) (bool, string, error) {
	inspect, err := d.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return false, "", fmt.Errorf("container '%s' not found: %w", containerName, err)
	}

	return inspect.State.Running, inspect.State.Status, nil
}

// UpdateResources dynamically adjusts CPU and RAM limits on a running container via cgroups
func (d *DockerClient) UpdateResources(ctx context.Context, containerName string, cpus float64, memoryMB int64) error {
	nanoCPUs := int64(cpus * 1e9)
	memoryBytes := memoryMB * 1024 * 1024
	// In Docker, MemorySwap must be >= Memory to satisfy the kernel cgroup constraint
	memorySwapBytes := memoryBytes * 2

	updateConfig := container.UpdateConfig{
		Resources: container.Resources{
			NanoCPUs:   nanoCPUs,
			Memory:     memoryBytes,
			MemorySwap: memorySwapBytes,
		},
	}

	res, err := d.cli.ContainerUpdate(ctx, containerName, updateConfig)
	if err != nil {
		return fmt.Errorf("failed to update resources for container '%s': %w", containerName, err)
	}

	if len(res.Warnings) > 0 {
		for _, w := range res.Warnings {
			fmt.Printf("⚠️  Docker Warning (%s): %s\n", containerName, w)
		}
	}

	return nil
}

// GetMetrics takes a single point-in-time sample of container resource usage
func (d *DockerClient) GetMetrics(ctx context.Context, containerName string) (*ContainerMetrics, error) {
	// Stream=false gives us a single snapshot rather than an infinite stream
	resp, err := d.cli.ContainerStats(ctx, containerName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stats for '%s': %w", containerName, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats body: %w", err)
	}

	var stats DockerStatsJSON
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats json: %w", err)
	}

	// 1. Calculate CPU % (Standard Docker CLI algorithm)
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage) - float64(stats.PreCPUStats.SystemCPUUsage)
	onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = 1.0
	}

	var cpuPercent float64
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
	}

	// 2. Calculate RAM usage (excluding cached inactive file pages)
	memUsage := stats.MemoryStats.Usage
	if stats.MemoryStats.Stats.InactiveFile < memUsage {
		memUsage -= stats.MemoryStats.Stats.InactiveFile
	}

	usedMB := float64(memUsage) / (1024 * 1024)
	limitMB := float64(stats.MemoryStats.Limit) / (1024 * 1024)
	var memPercent float64
	if limitMB > 0 {
		memPercent = (usedMB / limitMB) * 100.0
	}

	return &ContainerMetrics{
		CPUPercent:       cpuPercent,
		MemoryUsedMB:     usedMB,
		MemoryLimitMB:    limitMB,
		MemoryPercent:    memPercent,
		ThrottledPeriods: stats.CPUStats.ThrottlingData.ThrottledPeriods,
		ThrottledTimeUs:  stats.CPUStats.ThrottlingData.ThrottledTime / 1000, // nanoseconds to microseconds
	}, nil
}
