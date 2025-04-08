package images

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/rancher/charts-build-scripts/pkg/filesystem"
	"github.com/rancher/charts-build-scripts/pkg/logger"
	"github.com/rancher/charts-build-scripts/pkg/options"
	"github.com/rancher/charts-build-scripts/pkg/regsync"
)

// CheckRCTags checks for any images that have RC tags
func CheckRCTags(repoRoot string) map[string][]string {

	// Get the release options from the release.yaml file
	releaseOptions := getReleaseOptions(repoRoot)
	logger.Log(slog.LevelInfo, "checking for RC tags in charts", slog.Any("releaseOptions", releaseOptions))

	rcImageTagMap := make(map[string][]string, 0)

	// Get required tags for all images
	imageTagMap, err := regsync.GenerateFilteredImageTagMap(releaseOptions)
	if err != nil {
		logger.Fatal(fmt.Errorf("failed to generate image tag map: %s", err).Error())
	}

	logger.Log(slog.LevelInfo, "checking for RC tags in all collected images")

	// Grab all images that contain RC tags
	for image := range imageTagMap {
		for _, tag := range imageTagMap[image] {
			if strings.Contains(tag, "-rc") {
				rcImageTagMap[image] = append(rcImageTagMap[image], tag)
			}
		}
	}

	return rcImageTagMap
}

// getReleaseOptions returns the release options from the release.yaml file
func getReleaseOptions(repoRoot string) options.ReleaseOptions {
	// Get the filesystem on the repo root
	repoFs := filesystem.GetFilesystem(repoRoot)

	// Load the release options from the release.yaml file
	releaseOptions, err := options.LoadReleaseOptionsFromFile(repoFs, "release.yaml")
	if err != nil {
		logger.Fatal(fmt.Errorf("unable to unmarshall release.yaml: %s", err).Error())
	}

	return releaseOptions
}
