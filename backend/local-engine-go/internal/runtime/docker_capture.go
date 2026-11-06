package runtime

import (
	"context"
	"fmt"
	"time"
)

// ReadCompleteLiquibaseOutput requires the exact planned Docker name and ID
// to identify a stopped executor before trusting its private capture. It
// deliberately never asks Docker for logs or reads the former host-directory
// argument. Private files share one bounded read and exact pre/post inspection.
// See liquibase-private-capture-ownership.md and C02/C04 in planning recovery.
func (r *DockerRuntime) ReadCompleteLiquibaseOutput(ctx context.Context, name, id, _ string, identity CaptureIdentity, maxStreamBytes int64) (CompleteCapture, error) {
	if r == nil || !createdContainerID.MatchString(id) {
		return CompleteCapture{}, fmt.Errorf("invalid exact Liquibase executor ID")
	}
	inspect := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		inspection, found, err := r.InspectNamedContainer(ctx, name)
		if err != nil {
			return err
		}
		if !found || inspection.ID != id || inspection.Status != "exited" || inspection.ExitCode != identity.ExitCode {
			return fmt.Errorf("exact Liquibase executor is not stopped with the bound exit")
		}
		return nil
	}
	if err := inspect(); err != nil {
		return CompleteCapture{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	capture, err := readCompleteLiquibaseCapture(containerCaptureFiles{ctx: ctx, runtime: r, id: id}, identity, maxStreamBytes)
	if err != nil {
		return CompleteCapture{}, err
	}
	if err := inspect(); err != nil {
		return CompleteCapture{}, err
	}
	if err := ctx.Err(); err != nil {
		return CompleteCapture{}, err
	}
	return capture, nil
}
