package filesystem

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/rancher/charts-build-scripts/pkg/logger"
)

// walkMap walks inputMap and calls the callback function on all map type nodes including the root node.
func walkMap(inputMap interface{}, callback func(map[interface{}]interface{})) {
	switch data := inputMap.(type) {
	case map[interface{}]interface{}:
		callback(data)
		for _, value := range data {
			walkMap(value, callback)
		}
	case []interface{}:
		for _, elem := range data {
			walkMap(elem, callback)
		}
	}
}

// WalkAssetsFolder walks over the assets folder, untars files if their name matches one of the filter values,
// stores the values.yaml content into a map and then iterates over the map to collect the image repo and tag values
// into another map.
func WalkFilteredAssetsFolder(ctx context.Context, imageTagMap, filter map[string][]string, chartsToIgnoreTags map[string]string) error {

	assetErrorMap := make(map[string]error)
	// Walk through the assets folder of the repo
	filepath.Walk("./assets/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error occurred while walking over the assets directory file %s:%s", path, err)
		}

		// Get the file name
		filename := info.Name()

		// Check if the file name ends with tgz ? since we only care about them
		// to untar them and check for values.yaml files.
		if strings.HasSuffix(filename, ".tgz") {
			valuesYamlMaps, err := DecodeValueYamlInTgz(context.Background(), path, []string{"values.yaml", "values.yml"})
			if err != nil {
				assetErrorMap[filename] = err
				return fmt.Errorf("error occurred while getting values yaml into map in %s:%s", path, err)
			}

			// Get the chart name and version from the filename
			chartName, chartVersion, err := getChartNameAndVersion(filename)
			if err != nil {
				assetErrorMap[filename] = err
				return err
			}

			// Iterate over the filter map to check if the chart name and version are in the filter map
			for chart, versions := range filter {
				if strings.Compare(chartName, chart) == 0 {
					for _, version := range versions {
						if strings.Compare(chartVersion, version) == 0 {
							logger.Log(ctx, slog.LevelInfo, "collecting images and tags for chart", slog.String("chartName", chartName), slog.String("chartVersion", chartVersion))

							// There can be multiple values yaml files for single chart. So, making a for loop.
							for _, valuesYaml := range valuesYamlMaps {

								// Collecting all images with the following notation in the values yaml files
								// reposoitory :
								// tag :
								walkMap(valuesYaml, func(inputMap map[interface{}]interface{}) {
									repository, ok := inputMap["repository"].(string)
									if !ok {
										return
									}
									// No string type assertion because some charts have float typed image tags
									tag, ok := inputMap["tag"]
									if !ok {
										return
									}

									// If the chart & tag are in the ignore charttags map, we ignore them
									for ignoreChartName, ignoreTag := range chartsToIgnoreTags {
										// find the chart name using the path variable
										if strings.Contains(path, fmt.Sprintf("/%s/", ignoreChartName)) {
											if tag == ignoreTag {
												return
											}
										}
									}

									// If the tag is already found, we don't append it again
									found := slices.Contains(imageTagMap[repository], fmt.Sprintf("%v", tag))
									if !found {
										imageTagMap[repository] = append(imageTagMap[repository], fmt.Sprintf("%v", tag))
									}
								})
							}
						}
					}
				}
			}
		}

		return nil
	})

	if len(assetErrorMap) > 0 {
		return fmt.Errorf("error occurred while walking over the assets directory: %v", assetErrorMap)
	}

	return nil
}

// getChartNameAndVersion returns the chart name and version from the filename
func getChartNameAndVersion(filename string) (string, string, error) {
	// Remove the .tgz suffix
	if strings.HasSuffix(filename, ".tgz") {
		filename = filename[:len(filename)-4]
	} else {
		return "", "", fmt.Errorf("file does not have a .tgz suffix")
	}

	// Find the first digit from the left
	for i := 0; i < len(filename); i++ {
		if unicode.IsDigit(rune(filename[i+1])) {
			return filename[:i], filename[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("could not extract details from filename")
}
