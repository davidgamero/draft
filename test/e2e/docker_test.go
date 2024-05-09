package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
)

// DockerBuildAndPush calls the local docker daemon via dockerCli to build the repository imageName, using the dockerContextDir
func DockerBuildAndPush(ctx context.Context, dockerCli *client.Client, imageName string, dockerContextDir string) error {
	tar, err := archive.TarWithOptions(dockerContextDir, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("archiving dockerfile context: %s", err.Error())
	}

	res, err := dockerCli.ImageBuild(ctx, tar, types.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Tags:       []string{imageName},
	})
	if err != nil {
		return fmt.Errorf("starting docker image '%s' build: %s", imageName, err.Error())
	}
	scanner := bufio.NewScanner(res.Body)
	defer res.Body.Close()
	for scanner.Scan() {
		lastLine := scanner.Text()
		fmt.Println(lastLine)

		errLine := &ErrorLine{}
		json.Unmarshal([]byte(lastLine), errLine)
		if errLine.Error != "" {
			return fmt.Errorf("building docker image: %s", errLine.Error)
		}
	}

	pushResp, err := dockerCli.ImagePush(ctx, imageName, image.PushOptions{
		All:          true,
		RegistryAuth: "none",
	})
	if err != nil {
		return fmt.Errorf("starting push for image %s: %s", imageName, err.Error())
	}
	defer pushResp.Close()
	scanner = bufio.NewScanner(pushResp)
	for scanner.Scan() {
		lastLine := scanner.Text()
		fmt.Println(lastLine)

		errLine := &ErrorLine{}
		json.Unmarshal([]byte(lastLine), errLine)
		if errLine.Error != "" {
			return fmt.Errorf("pushing image %s: %s", imageName, errLine.Error)
		}
	}

	return nil
}
