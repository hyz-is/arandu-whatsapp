package message

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	himage "github.com/arandu-io/hesape/image"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/process"
)

const (
	defaultThumbnailMaxWidth    = 320
	defaultThumbnailMaxHeight   = 320
	defaultThumbnailJPEGQuality = 75
	defaultThumbnailMaxInput    = 50 * 1024 * 1024
	defaultThumbnailTimeout     = 15 * time.Second
	defaultFFmpegPath           = "ffmpeg"
)

var ErrThumbnailFailed = errors.New("thumbnail generation failed")

type Thumbnail struct {
	Bytes  []byte
	Width  int
	Height int
}

type ThumbnailConfig struct {
	MaxWidth      int
	MaxHeight     int
	JPEGQuality   int
	MaxInputBytes int64
	Timeout       time.Duration
	FFmpegPath    string
	TempDir       string
}

type ThumbnailService interface {
	FromImage(ctx context.Context, media []byte) (Thumbnail, error)
	FromVideo(ctx context.Context, media []byte) (Thumbnail, error)
}

type thumbnailService struct {
	config    ThumbnailConfig
	images    *himage.ImageManager
	processes *process.Factory
}

func DefaultThumbnailConfig() ThumbnailConfig {
	return ThumbnailConfig{
		MaxWidth:      defaultThumbnailMaxWidth,
		MaxHeight:     defaultThumbnailMaxHeight,
		JPEGQuality:   defaultThumbnailJPEGQuality,
		MaxInputBytes: defaultThumbnailMaxInput,
		Timeout:       defaultThumbnailTimeout,
		FFmpegPath:    "ffmpeg",
	}
}

func NewThumbnailService(config ThumbnailConfig) ThumbnailService {
	config = normalizeThumbnailConfig(config)
	return &thumbnailService{
		config:    config,
		images:    himage.NewImageManager(),
		processes: process.NewFactory(),
	}
}

func normalizeThumbnailConfig(config ThumbnailConfig) ThumbnailConfig {
	if config.MaxWidth <= 0 {
		config.MaxWidth = defaultThumbnailMaxWidth
	}
	if config.MaxHeight <= 0 {
		config.MaxHeight = defaultThumbnailMaxHeight
	}
	if config.JPEGQuality <= 0 || config.JPEGQuality > 100 {
		config.JPEGQuality = defaultThumbnailJPEGQuality
	}
	if config.MaxInputBytes <= 0 {
		config.MaxInputBytes = defaultThumbnailMaxInput
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultThumbnailTimeout
	}
	if config.FFmpegPath == "" {
		config.FFmpegPath = defaultFFmpegPath
	}
	return config
}

func (s *thumbnailService) FromImage(ctx context.Context, media []byte) (Thumbnail, error) {
	if err := s.validateInput(media); err != nil {
		return Thumbnail{}, err
	}
	select {
	case <-ctx.Done():
		return Thumbnail{}, ctx.Err()
	default:
	}
	resized, err := s.images.FromBytes(media).Scale(s.config.MaxWidth, s.config.MaxHeight)
	if err != nil {
		return Thumbnail{}, fmt.Errorf("%w: resize image: %w", ErrThumbnailFailed, err)
	}
	output := resized.ToJpg().Quality(s.config.JPEGQuality)
	contents, err := output.ToBytes(ctx)
	if err != nil {
		return Thumbnail{}, fmt.Errorf("%w: encode jpeg: %w", ErrThumbnailFailed, err)
	}
	if len(contents) == 0 {
		return Thumbnail{}, fmt.Errorf("%w: empty jpeg output", ErrThumbnailFailed)
	}
	width, height, err := output.Dimensions(ctx)
	if err != nil {
		return Thumbnail{}, fmt.Errorf("%w: read output dimensions: %w", ErrThumbnailFailed, err)
	}
	return Thumbnail{
		Bytes:  contents,
		Width:  width,
		Height: height,
	}, nil
}

func (s *thumbnailService) FromVideo(ctx context.Context, media []byte) (Thumbnail, error) {
	if err := s.validateInput(media); err != nil {
		return Thumbnail{}, err
	}
	dir, err := os.MkdirTemp(s.config.TempDir, "arandu-whatsapp-thumbnail-*")
	if err != nil {
		return Thumbnail{}, fmt.Errorf("%w: temp dir: %w", ErrThumbnailFailed, err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			hlog.For(ctx).DebugContext(ctx, "failed to remove thumbnail temp dir",
				"component", "thumbnail_service", "error", removeErr, "dir", dir)
		}
	}()

	inputPath := filepath.Join(dir, "input.video")
	if err := os.WriteFile(inputPath, media, 0600); err != nil {
		return Thumbnail{}, fmt.Errorf("%w: write video: %w", ErrThumbnailFailed, err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	var lastErr error
	for index, seek := range []string{"0", "0.1", "0.5"} {
		select {
		case <-timeoutCtx.Done():
			return Thumbnail{}, timeoutCtx.Err()
		default:
		}
		outputPath := filepath.Join(dir, "frame-"+strconv.Itoa(index)+".jpg")
		if err := s.extractVideoFrame(timeoutCtx, inputPath, outputPath, seek); err != nil {
			lastErr = err
			continue
		}
		frame, err := os.ReadFile(outputPath)
		if err != nil {
			lastErr = fmt.Errorf("%w: read frame: %w", ErrThumbnailFailed, err)
			continue
		}
		thumbnail, err := s.FromImage(ctx, frame)
		if err != nil {
			lastErr = err
			continue
		}
		return thumbnail, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: no video frame extracted", ErrThumbnailFailed)
	}
	return Thumbnail{}, lastErr
}

func (s *thumbnailService) validateInput(media []byte) error {
	if len(media) == 0 {
		return fmt.Errorf("%w: empty input", ErrThumbnailFailed)
	}
	if int64(len(media)) > s.config.MaxInputBytes {
		return fmt.Errorf("%w: input too large", ErrThumbnailFailed)
	}
	return nil
}

func (s *thumbnailService) extractVideoFrame(ctx context.Context, inputPath, outputPath, seek string) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-ss", seek,
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "4",
		outputPath,
	}
	result, err := s.processes.NewPendingProcess().Forever().Run(ctx, append([]string{s.config.FFmpegPath}, args...), nil)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: ffmpeg frame at %s: %w", ErrThumbnailFailed, seek, err)
	}
	if result.Failed() {
		return fmt.Errorf("%w: ffmpeg frame at %s exited with code %d: %s", ErrThumbnailFailed, seek, result.ExitCode(), sanitizeCommandOutput([]byte(result.ErrorOutput())))
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return fmt.Errorf("%w: frame missing: %w", ErrThumbnailFailed, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%w: empty frame", ErrThumbnailFailed)
	}
	return nil
}
