package worker

import (
	"context"
	"strconv"
	"strings"

	pb "github.com/bleenco/abstruse/proto"
	"github.com/bleenco/abstruse/worker/docker"
	"github.com/docker/docker/api/types"
)

// StartJob starts job task.
func StartJob(task *pb.JobTask) error {
	name := task.GetName()
	cli, err := docker.GetClient()
	if err != nil {
		return err
	}

	var commands [][]string

	for _, taskcmd := range task.GetCommands() {
		splitted := strings.Split(taskcmd, " ")
		commands = append(commands, splitted)
	}

	resp, err := docker.CreateContainer(cli, name, task.GetImage(), []string{})
	if err != nil {
		return err
	}

	containerID := resp.ID

	if err := SendRunningStatus(name); err != nil {
		return err
	}

	ctx := context.Background()
	var exitCode int
	var lastCommand []string

	for _, command := range commands {
		if !docker.IsContainerRunning(cli, containerID) {
			if err := docker.StartContainer(cli, containerID); err != nil {
				return err
			}
		}

		text := "\033[33;1m" + strings.Join(append([]string{"==>"}, command...), " ") + "\033[0m"
		if err := Client.WriteContainerOutput(ctx, containerID, text); err != nil {
			return err
		}

		conn, execID, err := docker.Exec(cli, containerID, append([]string{"pty"}, command...))
		if err != nil {
			return err
		}

		if err := Client.StreamContainerOutput(ctx, conn, containerID); err != nil {
			return err
		}

		inspect, err := cli.ContainerExecInspect(ctx, execID)
		if err != nil {
			return err
		}

		lastCommand = command
		exitCode = inspect.ExitCode

		if exitCode != 0 {
			break
		}
	}

	if exitCode == 0 {
		text := "\n\033[32;1mThe command \"" + strings.Join(lastCommand, " ") + "\" exited with 0.\033[0m"
		if err := Client.WriteContainerOutput(ctx, containerID, text); err != nil {
			return err
		}
		if err := SendPassingStatus(name); err != nil {
			return err
		}
	} else if exitCode == 137 {
		text := "\n\033[31;1mJob stopped with exit code 137.\033[0m"
		if err := Client.WriteContainerOutput(ctx, containerID, text); err != nil {
			return err
		}
		if err := SendStoppedStatus(name); err != nil {
			return err
		}
	} else {
		text := "\n\033[31;1mThe command \"" + strings.Join(lastCommand, " ") + "\" exited with " + strconv.Itoa(exitCode) + ".\033[0m"
		if err := Client.WriteContainerOutput(ctx, containerID, text); err != nil {
			return err
		}
		if err := SendFailingStatus(name); err != nil {
			return err
		}
	}

	return cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}

// StopJob stops job task.
func StopJob(containerID string) error {
	cli, err := docker.GetClient()
	if err != nil {
		return err
	}

	return cli.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{Force: true})
}
