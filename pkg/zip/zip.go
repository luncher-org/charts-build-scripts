package zip

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/rancher/charts-build-scripts/pkg/filesystem"
	"github.com/rancher/charts-build-scripts/pkg/helm"
	"github.com/rancher/charts-build-scripts/pkg/logger"
	"github.com/rancher/charts-build-scripts/pkg/path"

	helmLoader "helm.sh/helm/v3/pkg/chart/loader"
)

// ArchiveCharts zips charts from charts/ into assets/. If the asset was re-ordered, it will also update charts/.
// If specificChart is provided, it will filter the set of charts that will be targeted for zipping.
// It will also not update an asset if its internal contents have not changed.
func ArchiveCharts(ctx context.Context, repoRoot string, specificChart string) error {
	repoFs := filesystem.GetFilesystem(repoRoot)
	foundChart := false
	zipChart := func(ctx context.Context, fs billy.Filesystem, helmChartPath string, isDir bool) error {
		if !isDir || len(strings.Split(helmChartPath, "/")) != 3 {
			// We expect to be at charts/{chart}/{version}
			return nil
		}
		chartVersionPath, err := filesystem.MovePath(ctx, helmChartPath, path.RepositoryChartsDir, "")
		if err != nil {
			return fmt.Errorf("unable to get chart version path for %s", helmChartPath)
		}
		if len(specificChart) > 0 {
			if specificChart != chartVersionPath && specificChart != filepath.Dir(chartVersionPath) {
				// scripts should only operate on current chart
				return nil
			}
		}
		foundChart = true
		chartAssetsDirpath := filepath.Join(path.RepositoryAssetsDir, filepath.Dir(chartVersionPath))
		tgzPath, err := helm.GenerateArchive(ctx, repoFs, fs, helmChartPath, chartAssetsDirpath, nil)
		if err != nil {
			return fmt.Errorf("encountered error while trying to update archive based on chart in %s: %s", chartVersionPath, err)
		}
		// Note: since we use helm package to zip charts, it's possible that the tgz file
		// that is created reorders the contents of Chart.yaml / requirements.yaml to be
		// alphabetical. Therefore, when zipping a chart we always need to unzip the finalized
		// chart(s) back to the charts/ directory, which is done by calling UnzipAsset after this.
		currentAsset, err := filesystem.MovePath(ctx, tgzPath, path.RepositoryAssetsDir, "")
		if err != nil {
			return err
		}
		if err := DumpAssets(ctx, repoRoot, currentAsset); err != nil {
			return fmt.Errorf("encountered error while trying to update chart %s based on %s: %s", chartVersionPath, tgzPath, err)
		}
		return nil
	}

	if err := filesystem.WalkDir(ctx, repoFs, path.RepositoryChartsDir, zipChart); err != nil {
		return fmt.Errorf("encountered error while trying to zip charts: %s", err)
	}
	if len(specificChart) > 0 && !foundChart {
		return fmt.Errorf("could not find chart at %s/%s in repository", path.RepositoryChartsDir, specificChart)
	}

	return nil
}

// DumpAssets unzips assets from assets/ into charts/.
// If specificAsset is provided, it will filter the set of assets that will be targeted for unzipping.
func DumpAssets(ctx context.Context, repoRoot string, specificAsset string) error {
	repoFs := filesystem.GetFilesystem(repoRoot)
	foundAsset := false
	unzipAsset := func(ctx context.Context, fs billy.Filesystem, tgzPath string, isDir bool) error {
		if isDir || len(strings.Split(tgzPath, "/")) != 3 || filepath.Ext(tgzPath) != ".tgz" {
			// We expect to be at assets/{chart}/{chart}-{version}.tgz
			return nil
		}
		assetPath, err := filesystem.MovePath(ctx, tgzPath, path.RepositoryAssetsDir, "")
		if err != nil {
			return fmt.Errorf("unable to get tgz path for %s", tgzPath)
		}
		if len(specificAsset) > 0 {
			if specificAsset != assetPath && specificAsset != filepath.Dir(assetPath) {
				// scripts should only operate on current asset
				return nil
			}
		}

		// Unarchive Tgz file
		logger.Log(ctx, slog.LevelInfo, "unarchiving", slog.String("tgzPath", tgzPath))

		// Get path to unarchive tgz to
		foundAsset = true
		absAssetPath := filesystem.GetAbsPath(fs, tgzPath)
		chart, err := helmLoader.Load(absAssetPath)
		if err != nil {
			return fmt.Errorf("could not load Helm chart: %s", err)
		}
		// Unarchive tgz
		chartChartsDirpath := filepath.Join(path.RepositoryChartsDir, chart.Metadata.Name, chart.Metadata.Version)
		// If we remove an overlay file, the file will not be removed from the charts directory if it already exists,
		// the easiest way to solve this problem is to clean the target directory before un-archiving the chart's package
		if err := filesystem.RemoveAll(fs, chartChartsDirpath); err != nil {
			return fmt.Errorf("failed to clean directory for charts at %s: %s", chartChartsDirpath, err)
		}
		defer filesystem.PruneEmptyDirsInPath(ctx, fs, chartChartsDirpath)
		if err := filesystem.UnarchiveTgz(ctx, fs, tgzPath, "", chartChartsDirpath, true); err != nil {
			return err
		}

		logger.Log(ctx, slog.LevelInfo, "generated chart", slog.String("chartChartsDirpath", chartChartsDirpath))
		return nil
	}

	if err := filesystem.WalkDir(ctx, repoFs, path.RepositoryAssetsDir, unzipAsset); err != nil {
		return fmt.Errorf("encountered error while trying to zip charts: %s", err)
	}

	if len(specificAsset) > 0 && !foundAsset {
		return fmt.Errorf("could not find asset at %s/%s in repository", path.RepositoryAssetsDir, specificAsset)
	}
	return nil
}
