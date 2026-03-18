package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/cri"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage OCI container images",
	Long: `Manage OCI container images via the CRI image service.

Available subcommands:
  pull    - Pull an image from a registry
  list    - List locally available images
  inspect - Inspect image details
  prune   - Remove unused images

Examples:
  misbah image pull docker.io/library/alpine:latest
  misbah image list
  misbah image inspect alpine:latest
  misbah image prune`,
}

var imagePullCmd = &cobra.Command{
	Use:   "pull <image-ref>",
	Short: "Pull an image from a registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runImagePull,
}

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List locally available images",
	RunE:  runImageList,
}

var imageInspectCmd = &cobra.Command{
	Use:   "inspect <image-ref>",
	Short: "Inspect image details",
	Args:  cobra.ExactArgs(1),
	RunE:  runImageInspect,
}

var imagePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove all images",
	RunE:  runImagePrune,
}

func init() {
	imageCmd.AddCommand(imagePullCmd)
	imageCmd.AddCommand(imageListCmd)
	imageCmd.AddCommand(imageInspectCmd)
	imageCmd.AddCommand(imagePruneCmd)

	rootCmd.AddCommand(imageCmd)
}

func createImageClient(cmd *cobra.Command) (*cri.Client, error) {
	endpoint := config.GetCRIEndpoint()
	if ep, _ := cmd.Flags().GetString("cri-endpoint"); ep != "" {
		endpoint = ep
	}
	return cri.NewClient(endpoint, logger)
}

func runImagePull(cmd *cobra.Command, args []string) error {
	imageRef := args[0]
	logger.Infof("Pulling image: %s", imageRef)

	client, err := createImageClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create CRI client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := client.PullImage(ctx, imageRef); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	logger.Infof("Image pulled successfully: %s", imageRef)
	return nil
}

func runImageList(cmd *cobra.Command, args []string) error {
	client, err := createImageClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create CRI client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	images, err := client.ListImages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	if len(images) == 0 {
		logger.Infof("No images found")
		return nil
	}

	logger.Infof("%-60s %-15s %s", "IMAGE", "SIZE", "ID")
	for _, img := range images {
		tag := "<none>"
		if len(img.RepoTags) > 0 {
			tag = img.RepoTags[0]
		}
		id := img.Id
		if len(id) > 19 {
			id = id[:19]
		}
		logger.Infof("%-60s %-15s %s", tag, formatSize(img.Size_), id)
	}

	logger.Infof("")
	logger.Infof("Total: %d images", len(images))
	return nil
}

func runImageInspect(cmd *cobra.Command, args []string) error {
	imageRef := args[0]
	logger.Infof("Inspecting image: %s", imageRef)

	client, err := createImageClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create CRI client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	img, err := client.ImageStatus(ctx, imageRef)
	if err != nil {
		return fmt.Errorf("failed to inspect image: %w", err)
	}

	if img == nil {
		return fmt.Errorf("image not found: %s", imageRef)
	}

	logger.Infof("")
	logger.Infof("Image Details:")
	logger.Infof("  ID: %s", img.Id)
	logger.Infof("  Size: %s", formatSize(img.Size_))
	if len(img.RepoTags) > 0 {
		logger.Infof("  Tags:")
		for _, tag := range img.RepoTags {
			logger.Infof("    - %s", tag)
		}
	}
	if len(img.RepoDigests) > 0 {
		logger.Infof("  Digests:")
		for _, digest := range img.RepoDigests {
			logger.Infof("    - %s", digest)
		}
	}

	return nil
}

func runImagePrune(cmd *cobra.Command, args []string) error {
	logger.Infof("Pruning images")

	client, err := createImageClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create CRI client: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	images, err := client.ListImages(ctx)
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	if len(images) == 0 {
		logger.Infof("No images to prune")
		return nil
	}

	removed := 0
	for _, img := range images {
		ref := img.Id
		if len(img.RepoTags) > 0 {
			ref = img.RepoTags[0]
		}
		if err := client.RemoveImage(ctx, ref); err != nil {
			logger.Warnf("Failed to remove %s: %v", ref, err)
			continue
		}
		removed++
		logger.Infof("Removed: %s", ref)
	}

	logger.Infof("Pruned %d images", removed)
	return nil
}

func formatSize(bytes uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
