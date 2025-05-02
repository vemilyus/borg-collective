package docker

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/model"
	"golang.org/x/sync/errgroup"
)

type dockerBackupAction struct {
	id     string
	d      *Docker
	node   *backupNode
	nodes  []*backupNode
	action func(self dockerBackupAction) error
}

func (a *dockerBackupAction) Id() string {
	return a.id
}

func (a *dockerBackupAction) Execute(self borg.Action) error {
	realSelf, ok := self.(*dockerBackupAction)
	if !ok {
		return fmt.Errorf("unexpected action type: %T", self)
	}

	return realSelf.action(*realSelf)
}

func (a *dockerBackupAction) SetId(id string) {
	a.id = id
}

func (d *Docker) ensureNeededStoppedAction(node *backupNode) (borg.Action, error) {
	count := len(node.NeededBy)
	if count == 0 {
		return nil, fmt.Errorf("nothing needs %s", node.Backup.ID)
	}

	return &dockerBackupAction{
		id:   rand.Text(),
		d:    d,
		node: node,
		action: func(self dockerBackupAction) error {
			eg, ctx := errgroup.WithContext(context.Background())
			eg.SetLimit(len(self.node.NeededBy))

			for _, dependent := range node.NeededBy {
				eg.Go(func() error {
					return self.d.ensureContainerStopped(ctx, dependent.Backup.ID)
				})
			}

			return eg.Wait()
		},
	}, nil
}

func (d *Docker) ensureContainerRunningAction(node *backupNode) borg.Action {
	return &dockerBackupAction{
		id:   rand.Text(),
		d:    d,
		node: node,
		action: func(self dockerBackupAction) error {
			return self.d.ensureContainerRunning(context.Background(), self.node.Backup.ID)
		},
	}
}

func (d *Docker) ensureAllContainersRunningAction(nodes []*backupNode) borg.Action {
	return &dockerBackupAction{
		id:    rand.Text(),
		d:     d,
		nodes: nodes,
		action: func(self dockerBackupAction) error {
			eg, ctx := errgroup.WithContext(context.Background())
			eg.SetLimit(len(self.nodes))

			for _, node := range self.nodes {
				eg.Go(func() error {
					return self.d.ensureContainerRunning(ctx, node.Backup.ID)
				})
			}

			return eg.Wait()
		},
	}
}

func (d *Docker) ensureContainerStoppedAction(node *backupNode) borg.Action {
	return &dockerBackupAction{
		id:   rand.Text(),
		d:    d,
		node: node,
		action: func(self dockerBackupAction) error {
			return self.d.ensureContainerStopped(context.Background(), self.node.Backup.ID)
		},
	}
}

func (d *Docker) createBackupWorkAction(node *backupNode) borg.Action {
	return &dockerBackupAction{
		id:   rand.Text(),
		d:    d,
		node: node,
		action: func(self dockerBackupAction) error {
			backupContainer := self.node.Backup
			if backupContainer.Exec != nil {
				return d.performExecBackup(self.node, self.Id())
			} else if len(backupContainer.Volumes) > 0 {
				return d.performVolumeBackup(self.node, self.Id())
			}

			return nil
		},
	}
}

func (d *Docker) performExecBackup(node *backupNode, actionId string) error {
	if node.Backup.Exec.Stdout {
		return d.performExecBackupWithInput(node, actionId)
	} else {
		return d.performExecBackupWithPaths(node, actionId)
	}
}

func (d *Docker) performExecBackupWithInput(node *backupNode, actionId string) error {
	backupContainer := node.Backup

	var dockerAttach types.HijackedResponse
	defer func() {
		if dockerAttach.Conn != nil {
			dockerAttach.Close()
		}
	}()

	action, _ := d.b.BuildArchiveStdoutAction(
		fmt.Sprintf("dc-%s-%s", node.Project.ProjectName, backupContainer.ServiceName),
		func(ctx context.Context) (io.Reader, error, chan error) {
			dockerExec, err := d.dc.ContainerExecCreate(
				ctx,
				backupContainer.ID,
				container.ExecOptions{
					Cmd:          backupContainer.Exec.Command,
					AttachStdout: true,
				},
			)

			if err != nil {
				return nil, err, nil
			}

			dockerAttach, err = d.dc.ContainerExecAttach(ctx, dockerExec.ID, container.ExecAttachOptions{})
			if err != nil {
				return nil, err, nil
			}

			err = d.dc.ContainerExecStart(ctx, dockerExec.ID, container.ExecStartOptions{})
			if err != nil {
				return nil, err, nil
			}

			errChan := make(chan error, 1)

			go func() {
				err = d.waitForExec(ctx, dockerExec.ID)
				if err != nil {
					errChan <- err
				}
			}()

			return dockerAttach.Reader, nil, errChan
		},
	)

	action.SetId(actionId)

	return action.Execute(action)
}

func (d *Docker) performExecBackupWithPaths(node *backupNode, actionId string) error {
	backupContainer := node.Backup

	ctx := context.Background()

	dockerExec, err := d.dc.ContainerExecCreate(
		ctx,
		backupContainer.ID,
		container.ExecOptions{Cmd: backupContainer.Exec.Command},
	)

	if err != nil {
		return err
	}

	err = d.dc.ContainerExecStart(ctx, dockerExec.ID, container.ExecStartOptions{})
	if err != nil {
		return err
	}

	err = d.waitForExec(ctx, dockerExec.ID)
	if err != nil {
		return err
	}

	sourcePaths := make([]string, len(backupContainer.Exec.Paths))
	for _, path := range backupContainer.Exec.Paths {
		sourcePath, err := findSourcePath(backupContainer, path)
		if err != nil {
			return err
		}

		sourcePaths = append(sourcePaths, sourcePath)
	}

	if len(sourcePaths) == 0 {
		log.Warn().Msgf("nothing to do for backupContainer %s", backupContainer.ID)
		return nil
	}

	action, _ := d.b.BuildArchivePathsAction(
		fmt.Sprintf("dc-%s-%s", node.Project.ProjectName, backupContainer.ServiceName),
		sourcePaths,
	)

	action.SetId(actionId)

	return action.Execute(action)
}

func (d *Docker) performVolumeBackup(node *backupNode, actionId string) error {
	backupContainer := node.Backup
	sourcePaths := make([]string, len(backupContainer.Volumes))
	for _, vol := range backupContainer.Volumes {
		if vol.Source != "" && vol.Mode != "ro" {
			sourcePaths = append(sourcePaths, vol.Source)
		}
	}

	if len(sourcePaths) == 0 {
		log.Warn().Msgf("nothing to do for backupContainer %s", backupContainer.ID)
		return nil
	}

	baseName := fmt.Sprintf("dc-%s-%s", node.Project.ProjectName, backupContainer.ServiceName)
	action, err := d.b.BuildArchivePathsAction(baseName, sourcePaths)
	if err != nil {
		return err
	}

	action.SetId(actionId)

	return action.Execute(action)
}

func findSourcePath(backupContainer model.ContainerBackup, path string) (string, error) {
	for _, vol := range backupContainer.AllVolumes {
		if strings.HasPrefix(path, vol.Destination) {
			return fmt.Sprintf("%s%s", vol.Source, path[len(vol.Destination):]), nil
		}
	}

	if backupContainer.UpperDirPath == "" {
		return "", fmt.Errorf("no upper dir path known for container %s", backupContainer.ID)
	}

	return fmt.Sprintf("%s%s", backupContainer.UpperDirPath, path), nil
}
