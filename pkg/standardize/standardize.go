package standardize

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/rancher/charts-build-scripts/pkg/filesystem"
	"github.com/rancher/charts-build-scripts/pkg/helm"
	"github.com/rancher/charts-build-scripts/pkg/logger"
	"github.com/rancher/charts-build-scripts/pkg/path"
	"github.com/rancher/charts-build-scripts/pkg/zip"

	helmChart "helm.sh/helm/v3/pkg/chart"
	helmLoader "helm.sh/helm/v3/pkg/chart/loader"
)

// RestructureChartsAndAssets takes in a Helm repository and restructures the contents of assets/ based on the contents of charts/
// It then dumps the created assets/ back into charts/ and regenerates the Helm index.
// As a result, the final outputted Helm repository can now be used by the charts-build-scripts as it has been standardized.
func RestructureChartsAndAssets(ctx context.Context, repoFs billy.Filesystem) error {
	exists, err := filesystem.PathExists(ctx, repoFs, path.RepositoryChartsDir)
	if err != nil {
		return fmt.Errorf("encountered error while checking if %s exists: %s", path.RepositoryChartsDir, err)
	}
	if !exists {
		return fmt.Errorf("could not find charts in repository rooted at %s", repoFs.Root())
	}
	return standardizeAssetsFromCharts(ctx, repoFs)
}

func standardizeAssetsFromCharts(ctx context.Context, repoFs billy.Filesystem) error {
	// Collect all valid charts from charts directory
	targetChartPaths := make(map[string]*helmChart.Chart)
	collectAllValidCharts := func(ctx context.Context, fs billy.Filesystem, path string, isDir bool) error {
		if isDir {
			return nil
		}
		if filepath.Base(path) == "Chart.yaml" {
			// found a valid chart
			var err error
			chartPath := filepath.Dir(path)
			chart, err := helmLoader.Load(filesystem.GetAbsPath(fs, chartPath))
			if err != nil {
				return err
			}
			targetChartPaths[chartPath] = chart
		}
		return nil
	}
	// Collect all charts from charts directory
	logger.Log(ctx, slog.LevelInfo, "collecting valid charts", slog.String("RepositoryChartsDir", path.RepositoryChartsDir))

	if err := filesystem.WalkDir(ctx, repoFs, path.RepositoryChartsDir, collectAllValidCharts); err != nil {
		return fmt.Errorf("encountered error while trying to find Helm charts in repository: %s", err)
	}
	// Ensure you do not collect subcharts defined within charts
	logger.Log(ctx, slog.LevelInfo, "removing collected subcharts")
	for chartPath := range targetChartPaths {
		chartPathDir := chartPath
		for {
			chartPathDir = filepath.Dir(chartPathDir)
			if chartPathDir == "." {
				logger.Log(ctx, slog.LevelDebug, "identified chart", slog.String("chartPath", chartPath))
				break
			}
			_, ok := targetChartPaths[chartPathDir]
			if ok {
				// Identified a subchart
				delete(targetChartPaths, chartPath)
				break
			}
		}
	}
	// Ensure that charts names + versions are unique
	logger.Log(ctx, slog.LevelInfo, "ensuring chart versions are unique")
	targetChartsVersions := make(map[string]string)
	for chartPath, chart := range targetChartPaths {
		chartVersion := fmt.Sprintf("%s-%s", chart.Metadata.Name, chart.Metadata.Version)
		currChartPath, ok := targetChartsVersions[chartVersion]
		if !ok {
			targetChartsVersions[chartVersion] = chartPath
			continue
		}

		logger.Log(ctx, slog.LevelError, "chart version conflict", slog.String("chart.Metadata.Name", chart.Metadata.Name), slog.String("chart.Metadata.Version", chart.Metadata.Version))
		logger.Log(ctx, slog.LevelError, "chart version conflict", slog.String("currChartPath", currChartPath), slog.String("chartPath", chartPath))
		return fmt.Errorf("chart %s at version %s is declared in %s and %s", chart.Metadata.Name, chart.Metadata.Version, currChartPath, chartPath)
	}
	// Archive charts into assets
	if err := filesystem.RemoveAll(repoFs, path.RepositoryAssetsDir); err != nil {
		return fmt.Errorf("unable to remove all assets to reconstruct directory: %s", err)
	}
	for chartPath, chart := range targetChartPaths {
		chartAssetsDirpath := filepath.Join(path.RepositoryAssetsDir, chart.Metadata.Name)
		if _, err := helm.GenerateArchive(ctx, repoFs, repoFs, chartPath, chartAssetsDirpath, nil); err != nil {
			return fmt.Errorf("encountered error while trying to update archive based on chart in %s: %s", chartPath, err)
		}
	}
	// Remove charts because all of them will be reconstructed
	if err := filesystem.RemoveAll(repoFs, path.RepositoryChartsDir); err != nil {
		return fmt.Errorf("unable to remove all assets to reconstruct directory: %s", err)
	}
	// Reconstruct charts
	if err := zip.DumpAssets(ctx, repoFs.Root(), ""); err != nil {
		return fmt.Errorf("encountered error while trying to dump Helm charts in repository: %s", err)
	}
	// Reconstruct index.yaml
	if err := helm.CreateOrUpdateHelmIndex(ctx, repoFs); err != nil {
		return fmt.Errorf("encountered error while trying to recreate Helm index: %s", err)
	}
	return nil
}
