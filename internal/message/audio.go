package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/arandu-io/framework/security"
	httpclient "github.com/arandu-io/hesape/http/client"
	hlog "github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/process"
	"go.mau.fi/whatsmeow"
	wae2e "go.mau.fi/whatsmeow/proto/waE2E"
	watypes "go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	dbtypes "github.com/hyz-is/arandu-whatsapp/internal/database/types"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp"
	"github.com/hyz-is/arandu-whatsapp/internal/whatsapp/address"
)

const (
	audioOutputMIME      = "audio/ogg; codecs=opus"
	audioOutputFilename  = "audio.ogg"
	maxDownloadRedirects = 5
)

type AudioConfig struct {
	MaxInputBytes      int64
	MaxDurationSeconds uint32
	DownloadTimeout    time.Duration
	ProcessingTimeout  time.Duration
	OpusBitrate        string
	SampleRate         int
	Channels           int
	FFmpegPath         string
	FFprobePath        string
	TempDir            string
}

func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		MaxInputBytes:      50 * 1024 * 1024,
		MaxDurationSeconds: 60 * 60,
		DownloadTimeout:    30 * time.Second,
		ProcessingTimeout:  60 * time.Second,
		OpusBitrate:        "32k",
		SampleRate:         48000,
		Channels:           1,
		FFmpegPath:         "ffmpeg",
		FFprobePath:        "ffprobe",
	}
}

type PreparedAudio struct {
	Data            []byte
	MIMEType        string
	Filename        string
	DurationSeconds uint32
	Codec           string
	Container       string
}

type AudioProcessor interface {
	PreparePTT(ctx context.Context, input []byte, filename string, mimeType string) (PreparedAudio, error)
}

type FFmpegAudioProcessor struct {
	config    AudioConfig
	processes *process.Factory
}

func NewFFmpegAudioProcessor(config AudioConfig) *FFmpegAudioProcessor {
	defaults := DefaultAudioConfig()
	if config.MaxInputBytes <= 0 {
		config.MaxInputBytes = defaults.MaxInputBytes
	}
	if config.MaxDurationSeconds == 0 {
		config.MaxDurationSeconds = defaults.MaxDurationSeconds
	}
	if config.DownloadTimeout <= 0 {
		config.DownloadTimeout = defaults.DownloadTimeout
	}
	if config.ProcessingTimeout <= 0 {
		config.ProcessingTimeout = defaults.ProcessingTimeout
	}
	if config.OpusBitrate == "" {
		config.OpusBitrate = defaults.OpusBitrate
	}
	if config.SampleRate <= 0 {
		config.SampleRate = defaults.SampleRate
	}
	if config.Channels <= 0 {
		config.Channels = defaults.Channels
	}
	if strings.TrimSpace(config.FFmpegPath) == "" {
		config.FFmpegPath = defaults.FFmpegPath
	}
	if strings.TrimSpace(config.FFprobePath) == "" {
		config.FFprobePath = defaults.FFprobePath
	}
	return &FFmpegAudioProcessor{
		config:    config,
		processes: process.NewFactory(),
	}
}

func (p *FFmpegAudioProcessor) PreparePTT(ctx context.Context, input []byte, filename string, mimeType string) (PreparedAudio, error) {
	if len(input) == 0 {
		return PreparedAudio{}, fmt.Errorf("%w: empty audio", ErrInvalidRequest)
	}
	if int64(len(input)) > p.config.MaxInputBytes {
		return PreparedAudio{}, ErrPayloadTooLarge
	}
	if !looksLikeSupportedAudio(mimeType, filename) {
		return PreparedAudio{}, ErrUnsupportedMediaType
	}
	processCtx, cancel := context.WithTimeout(ctx, p.config.ProcessingTimeout)
	defer cancel()

	tempDir, err := os.MkdirTemp(p.config.TempDir, "arandu-whatsapp-audio-*")
	if err != nil {
		return PreparedAudio{}, fmt.Errorf("%w: temp dir: %w", ErrAudioProcessing, err)
	}
	defer os.RemoveAll(tempDir)

	inputName := safeFilename(filename)
	if inputName == "" {
		inputName = "input"
	}
	inputPath := filepath.Join(tempDir, "input-"+inputName)
	outputPath := filepath.Join(tempDir, audioOutputFilename)
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		return PreparedAudio{}, fmt.Errorf("%w: write input: %w", ErrAudioProcessing, err)
	}

	probe, err := p.probeAudio(processCtx, inputPath)
	if err != nil {
		return PreparedAudio{}, fmt.Errorf("%w: probe input: %w", ErrUnsupportedMediaType, err)
	}
	if !probe.isAudio() {
		return PreparedAudio{}, ErrUnsupportedMediaType
	}

	var output []byte
	var finalProbe audioProbe
	if probe.isOggOpus() {
		output = input
		finalProbe = probe
	} else {
		if err := p.convertToOggOpus(processCtx, inputPath, outputPath); err != nil {
			return PreparedAudio{}, fmt.Errorf("%w: ffmpeg: %w", ErrAudioProcessing, err)
		}
		output, err = os.ReadFile(outputPath)
		if err != nil {
			return PreparedAudio{}, fmt.Errorf("%w: read output: %w", ErrAudioProcessing, err)
		}
		finalProbe, err = p.probeAudio(processCtx, outputPath)
		if err != nil {
			return PreparedAudio{}, fmt.Errorf("%w: probe output: %w", ErrAudioProcessing, err)
		}
	}
	seconds, err := normalizeDuration(finalProbe.Duration, p.config.MaxDurationSeconds)
	if err != nil {
		return PreparedAudio{}, err
	}
	if int64(len(output)) > p.config.MaxInputBytes {
		return PreparedAudio{}, ErrPayloadTooLarge
	}
	return PreparedAudio{
		Data:            output,
		MIMEType:        audioOutputMIME,
		Filename:        audioOutputFilename,
		DurationSeconds: seconds,
		Codec:           "opus",
		Container:       "ogg",
	}, nil
}

func (p *FFmpegAudioProcessor) convertToOggOpus(ctx context.Context, inputPath string, outputPath string) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-vn",
		"-ac", strconv.Itoa(p.config.Channels),
		"-ar", strconv.Itoa(p.config.SampleRate),
		"-c:a", "libopus",
		"-b:a", p.config.OpusBitrate,
		"-vbr", "on",
		"-application", "voip",
		"-f", "ogg",
		outputPath,
	}
	result, err := p.processes.NewPendingProcess().Forever().Run(ctx, append([]string{p.config.FFmpegPath}, args...), nil)
	if err != nil {
		return err
	}
	if result.Failed() {
		return fmt.Errorf("exit code %d: %s", result.ExitCode(), sanitizeCommandOutput([]byte(result.ErrorOutput())))
	}
	return nil
}

type audioProbe struct {
	Duration float64
	Codec    string
	Format   string
}

func (p audioProbe) isAudio() bool {
	return p.Codec != ""
}

func (p audioProbe) isOggOpus() bool {
	return strings.EqualFold(p.Codec, "opus") && strings.Contains(strings.ToLower(p.Format), "ogg")
}

func (p *FFmpegAudioProcessor) probeAudio(ctx context.Context, path string) (audioProbe, error) {
	formatResult, err := p.processes.NewPendingProcess().Forever().Run(ctx, []string{
		p.config.FFprobePath, "-v", "error", "-show_entries", "format=duration,format_name", "-of", "json", path,
	}, nil)
	if err != nil {
		return audioProbe{}, err
	}
	if formatResult.Failed() {
		return audioProbe{}, fmt.Errorf("ffprobe format exited with code %d: %s", formatResult.ExitCode(), sanitizeCommandOutput([]byte(formatResult.ErrorOutput())))
	}
	streamResult, err := p.processes.NewPendingProcess().Forever().Run(ctx, []string{
		p.config.FFprobePath, "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,duration", "-of", "json", path,
	}, nil)
	if err != nil {
		return audioProbe{}, err
	}
	if streamResult.Failed() {
		return audioProbe{}, fmt.Errorf("ffprobe stream exited with code %d: %s", streamResult.ExitCode(), sanitizeCommandOutput([]byte(streamResult.ErrorOutput())))
	}
	var format struct {
		Format struct {
			Duration   string `json:"duration"`
			FormatName string `json:"format_name"`
		} `json:"format"`
	}
	var stream struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			Duration  string `json:"duration"`
		} `json:"streams"`
	}
	_ = json.Unmarshal([]byte(formatResult.Output()), &format)
	_ = json.Unmarshal([]byte(streamResult.Output()), &stream)
	probe := audioProbe{Format: format.Format.FormatName}
	if len(stream.Streams) > 0 {
		probe.Codec = stream.Streams[0].CodecName
		probe.Duration, _ = strconv.ParseFloat(stream.Streams[0].Duration, 64)
	}
	if probe.Duration <= 0 {
		probe.Duration, _ = strconv.ParseFloat(format.Format.Duration, 64)
	}
	if probe.Codec == "" {
		return audioProbe{}, ErrUnsupportedMediaType
	}
	return probe, nil
}

func normalizeDuration(value float64, maxSeconds uint32) (uint32, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, ErrInvalidAudioDuration
	}
	rounded := math.Ceil(value)
	if rounded > math.MaxUint32 {
		return 0, ErrInvalidAudioDuration
	}
	if maxSeconds > 0 && rounded > float64(maxSeconds) {
		return 0, ErrInvalidAudioDuration
	}
	return uint32(rounded), nil
}

var supportedAudioMIMEs = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp3":   true,
	"audio/mp4":   true,
	"audio/aac":   true,
	"audio/x-m4a": true,
	"audio/ogg":   true,
	"audio/opus":  true,
	"audio/wav":   true,
	"audio/x-wav": true,
	"audio/flac":  true,
}

var supportedAudioExtensions = map[string]bool{
	".mp3":  true,
	".mp4":  true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".flac": true,
}

func looksLikeSupportedAudio(mimeType string, filename string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if supportedAudioMIMEs[normalized] {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	return supportedAudioExtensions[ext]
}

func safeFilename(original string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(original), "\\", "/")
	base := filepath.Base(normalized)
	if base == "." || base == "/" {
		return ""
	}
	var builder strings.Builder
	for _, r := range base {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			continue
		}
		builder.WriteRune(r)
	}
	name := strings.TrimSpace(builder.String())
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

func sanitizeCommandOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 512 {
		return text[:512]
	}
	return text
}

type audioSource struct {
	Source       string
	Data         []byte
	MIMEType     string
	Filename     string
	OriginalURL  string
	OriginalSize int
}

func (s *MessageService) SendWhatsAppAudio(ctx context.Context, grant security.Grant, instanceName string, input SendWhatsAppAudioRequest) (SendResult, error) {
	if strings.TrimSpace(input.Number) == "" {
		return SendResult{}, fmt.Errorf("%w: number is required", ErrInvalidRequest)
	}
	if input.AudioMessage == nil || strings.TrimSpace(input.AudioMessage.Audio) == "" {
		return SendResult{}, fmt.Errorf("%w: audioMessage.audio is required", ErrInvalidRequest)
	}
	audioURL, err := validateHTTPURL(input.AudioMessage.Audio)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: audioMessage.audio", ErrInvalidRequest)
	}
	data, mimeType, filename, err := s.downloadAudio(ctx, audioURL)
	if err != nil {
		return SendResult{}, err
	}
	return s.sendWhatsAppAudio(ctx, grant, instanceName, input.Number, audioSource{
		Source:       "url",
		Data:         data,
		MIMEType:     mimeType,
		Filename:     filename,
		OriginalURL:  audioURL,
		OriginalSize: len(data),
	}, input.Options)
}

func (s *MessageService) SendWhatsAppAudioFile(ctx context.Context, grant security.Grant, instanceName string, number string, file multipart.File, header *multipart.FileHeader, options *MessageOptions) (SendResult, error) {
	if strings.TrimSpace(number) == "" {
		return SendResult{}, fmt.Errorf("%w: number is required", ErrInvalidRequest)
	}
	if file == nil || header == nil {
		return SendResult{}, fmt.Errorf("%w: attachment is required", ErrInvalidRequest)
	}
	filename := safeFilename(header.Filename)
	if filename == "" {
		filename = "audio"
	}
	maxBytes := DefaultAudioConfig().MaxInputBytes
	if header.Size > maxBytes {
		return SendResult{}, ErrPayloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: read attachment", ErrInvalidRequest)
	}
	if int64(len(data)) > maxBytes {
		return SendResult{}, ErrPayloadTooLarge
	}
	if len(data) == 0 {
		return SendResult{}, fmt.Errorf("%w: empty attachment", ErrInvalidRequest)
	}
	mimeType := detectAudioMIME(data, header.Header.Get("Content-Type"))
	return s.sendWhatsAppAudio(ctx, grant, instanceName, number, audioSource{
		Source:       "upload",
		Data:         data,
		MIMEType:     mimeType,
		Filename:     filename,
		OriginalSize: len(data),
	}, options)
}

func (s *MessageService) sendWhatsAppAudio(ctx context.Context, grant security.Grant, instanceName string, number string, source audioSource, options *MessageOptions) (SendResult, error) {
	instance, err := s.authenticateInstance(ctx, grant, instanceName)
	if err != nil {
		return SendResult{}, err
	}
	presence, delay, err := validateAudioOptions(options)
	if err != nil {
		return SendResult{}, err
	}
	quoted, err := s.resolveAudioQuoted(ctx, grant, instance.Instance.ID, options)
	if err != nil {
		return SendResult{}, err
	}
	managed, err := s.clients.ResolveConnectedClient(ctx, grant, instance.Instance.Name)
	if err != nil {
		return SendResult{}, err
	}
	if managed == nil || managed.Client == nil || !managed.IsReady() {
		return SendResult{}, whatsapp.ErrClientNotConnected
	}
	if s.resolver == nil {
		return SendResult{}, fmt.Errorf("%w: address resolver unavailable", ErrRecipientInvalid)
	}
	resolved, err := s.resolver.Resolve(ctx, grant, managed.Client, address.ResolveInput{
		InstanceID: instance.Instance.ID,
		Address:    number,
	})
	if err != nil {
		return SendResult{}, err
	}
	recipient := resolved.CanonicalJID
	if mentionAllEnabled(options) {
		sourceCopy := audioSource{
			Source:       source.Source,
			Data:         append([]byte(nil), source.Data...),
			MIMEType:     source.MIMEType,
			Filename:     source.Filename,
			OriginalURL:  source.OriginalURL,
			OriginalSize: source.OriginalSize,
		}
		return s.enqueueMentionAll(ctx, grant, instance.Instance, managed.Client, recipient, outboundRequest{
			Recipient: recipientInput(&number, nil, nil),
			Options:   options,
			Kind:      KindAudio,
			Build: func(ctx context.Context, client *whatsmeow.Client, quoted *wae2e.ContextInfo) (*wae2e.Message, string, map[string]any, error) {
				processor := s.audio
				if processor == nil {
					processor = NewFFmpegAudioProcessor(DefaultAudioConfig())
				}
				prepared, err := processor.PreparePTT(ctx, sourceCopy.Data, sourceCopy.Filename, sourceCopy.MIMEType)
				if err != nil {
					return nil, "", nil, err
				}
				upload, err := client.Upload(ctx, prepared.Data, whatsmeow.MediaAudio)
				if err != nil {
					return nil, "", nil, fmt.Errorf("%w: %w", ErrUploadFailed, err)
				}
				message, content := buildPTTAudioMessage(upload, prepared, quoted, sourceCopy)
				return message, "audioMessage", content, nil
			},
		}, quoted, presence, delay)
	}
	processor := s.audio
	if processor == nil {
		processor = NewFFmpegAudioProcessor(DefaultAudioConfig())
	}
	prepared, err := processor.PreparePTT(ctx, source.Data, source.Filename, source.MIMEType)
	if err != nil {
		return SendResult{}, err
	}
	upload, err := managed.Client.Upload(ctx, prepared.Data, whatsmeow.MediaAudio)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %w", ErrUploadFailed, err)
	}
	protoMessage, content := buildPTTAudioMessage(upload, prepared, quoted, source)

	hlog.For(ctx).InfoContext(ctx, "sending WhatsApp PTT audio",
		"component", "message_service",
		"operation", "message.audio.send",
		"instance_id", instance.Instance.ID,
		"instance_name", instance.Instance.Name,
		"remote_jid", address.MaskAddress(recipient.String()),
		"source", source.Source,
		"original_mime_type", source.MIMEType,
		"output_mime_type", prepared.MIMEType,
		"original_size", source.OriginalSize,
		"output_size", len(prepared.Data),
		"duration_seconds", prepared.DurationSeconds,
		"codec", prepared.Codec,
		"container", prepared.Container,
		"delay_ms", delay.Milliseconds(),
	)

	if err := applyAudioPresenceAndDelay(ctx, managed.Client, recipient, presence, delay); err != nil {
		return SendResult{}, err
	}
	sendResp, err := managed.Client.SendMessage(ctx, recipient, protoMessage)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: %w", ErrSendFailed, err)
	}
	content = SanitizeMessageContent(content).(map[string]any)
	raw, err := json.Marshal(content)
	if err != nil {
		return SendResult{}, fmt.Errorf("%w: marshal audio content: %w", ErrInvalidRequest, err)
	}
	remote := recipient.String()
	isGroup := recipient.Server == watypes.GroupServer
	timestamp := int32(sendResp.Timestamp.Unix())
	if timestamp <= 0 {
		timestamp = int32(time.Now().Unix())
	}
	persisted, err := s.messages.Create(ctx, grant, dbtypes.CreateMessageInput{
		KeyID:            string(sendResp.ID),
		KeyRemoteJid:     &remote,
		KeyFromMe:        true,
		MessageType:      "audioMessage",
		Content:          raw,
		MessageTimestamp: timestamp,
		Device:           dbtypes.DeviceMessageWeb,
		IsGroup:          &isGroup,
		InstanceID:       instance.Instance.ID,
	})
	if err != nil {
		hlog.For(ctx).ErrorContext(ctx, "PTT audio sent but persistence failed",
			"component", "message_service",
			"error", err,
			"key_id", string(sendResp.ID),
			"instance_id", instance.Instance.ID,
			"key_remote_jid", address.MaskAddress(remote),
		)
		return SendResult{}, ErrPersistenceFailed
	}
	s.dispatchSendMessageWebhook(ctx, grant, instance.Instance, persisted)
	return SendResult{Message: persisted}, nil
}

func buildPTTAudioMessage(upload whatsmeow.UploadResponse, prepared PreparedAudio, quoted *wae2e.ContextInfo, source audioSource) (*wae2e.Message, map[string]any) {
	now := time.Now().Unix()
	audio := &wae2e.AudioMessage{
		URL:               proto.String(upload.URL),
		DirectPath:        proto.String(upload.DirectPath),
		MediaKey:          upload.MediaKey,
		Mimetype:          proto.String(prepared.MIMEType),
		FileEncSHA256:     upload.FileEncSHA256,
		FileSHA256:        upload.FileSHA256,
		FileLength:        proto.Uint64(uint64(len(prepared.Data))),
		Seconds:           proto.Uint32(prepared.DurationSeconds),
		PTT:               proto.Bool(true),
		ContextInfo:       quoted,
		MediaKeyTimestamp: proto.Int64(now),
	}
	content := map[string]any{
		"url":               upload.URL,
		"mimetype":          prepared.MIMEType,
		"fileLength":        strconv.FormatUint(uint64(len(prepared.Data)), 10),
		"seconds":           prepared.DurationSeconds,
		"ptt":               true,
		"fileSha256":        base64.StdEncoding.EncodeToString(upload.FileSHA256),
		"fileEncSha256":     base64.StdEncoding.EncodeToString(upload.FileEncSHA256),
		"mediaKey":          base64.StdEncoding.EncodeToString(upload.MediaKey),
		"directPath":        upload.DirectPath,
		"mediaKeyTimestamp": strconv.FormatInt(now, 10),
		"metadata": map[string]any{
			"source":           source.Source,
			"originalFilename": source.Filename,
			"outputFilename":   prepared.Filename,
			"originalMimeType": source.MIMEType,
			"codec":            prepared.Codec,
			"container":        prepared.Container,
		},
	}
	if quoted != nil {
		content["contextInfo"] = contextInfoContent(quoted)
	}
	return &wae2e.Message{AudioMessage: audio}, content
}

const audioMaxDelayMilliseconds int64 = 300000

func validateAudioOptions(options *MessageOptions) (*string, time.Duration, error) {
	defaultPresence := "recording"
	if options == nil {
		return &defaultPresence, 0, nil
	}
	var delay time.Duration
	if options.Delay != nil {
		if *options.Delay < 0 {
			return nil, 0, fmt.Errorf("%w: negative delay", ErrDelayInvalid)
		}
		if *options.Delay > audioMaxDelayMilliseconds {
			return nil, 0, fmt.Errorf("%w: delay too high", ErrDelayInvalid)
		}
		delay = time.Duration(*options.Delay) * time.Millisecond
	}
	presence := &defaultPresence
	if options.Presence != nil {
		normalized := strings.ToLower(strings.TrimSpace(*options.Presence))
		if normalized != "recording" && normalized != "paused" {
			return nil, 0, fmt.Errorf("%w: unsupported audio presence", ErrPresenceInvalid)
		}
		presence = &normalized
	}
	return presence, delay, nil
}

func applyAudioPresenceAndDelay(ctx context.Context, client *whatsmeow.Client, to watypes.JID, presence *string, delay time.Duration) error {
	if presence != nil && *presence == "recording" {
		if err := client.SendChatPresence(ctx, to, watypes.ChatPresenceComposing, watypes.ChatPresenceMediaAudio); err != nil {
			return fmt.Errorf("%w: set recording presence: %w", ErrSendFailed, err)
		}
		defer func() {
			_ = client.SendChatPresence(context.Background(), to, watypes.ChatPresencePaused, watypes.ChatPresenceMediaText)
		}()
	} else if presence != nil && *presence == "paused" {
		if err := client.SendChatPresence(ctx, to, watypes.ChatPresencePaused, watypes.ChatPresenceMediaText); err != nil {
			return fmt.Errorf("%w: set paused presence: %w", ErrSendFailed, err)
		}
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *MessageService) resolveAudioQuoted(ctx context.Context, grant security.Grant, instanceID int64, options *MessageOptions) (*wae2e.ContextInfo, error) {
	if options == nil {
		return nil, nil
	}
	if options.QuotedMessageID != nil {
		return s.resolveQuoted(ctx, grant, instanceID, options)
	}
	if options.QuotedMessage != nil {
		if len(options.QuotedMessage) == 0 {
			return nil, nil
		}
		return contextInfoFromMap(options.QuotedMessage)
	}
	return nil, nil
}

func detectAudioMIME(data []byte, header string) string {
	headerType := strings.TrimSpace(strings.Split(header, ";")[0])
	if supportedAudioMIMEs[strings.ToLower(headerType)] {
		return headerType
	}
	detected := http.DetectContentType(data)
	if supportedAudioMIMEs[strings.ToLower(detected)] {
		return detected
	}
	return headerType
}

func newAudioHTTPClient(factory *httpclient.Factory) *http.Client {
	if factory == nil {
		factory = httpclient.NewFactory(nil)
	}
	config := DefaultAudioConfig()
	return factory.
		MaxResponseBytes(config.MaxInputBytes + 1).
		CreatePendingRequest().
		Timeout(config.DownloadTimeout).
		MaxRedirects(maxDownloadRedirects).
		CreateClient(nil)
}

func (s *MessageService) downloadAudio(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	_, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: audio url", ErrInvalidRequest)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: request", ErrDownloadFailed)
	}
	client := s.audioHTTP
	if client == nil {
		client = newAudioHTTPClient(httpclient.NewFactory(nil))
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, httpclient.ErrResponseTooLarge) {
			return nil, "", "", ErrPayloadTooLarge
		}
		return nil, "", "", fmt.Errorf("%w: %w", ErrDownloadFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", "", fmt.Errorf("%w: status %d", ErrDownloadFailed, resp.StatusCode)
	}
	maxBytes := DefaultAudioConfig().MaxInputBytes
	if resp.ContentLength > maxBytes {
		return nil, "", "", ErrPayloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		if errors.Is(err, httpclient.ErrResponseTooLarge) {
			return nil, "", "", ErrPayloadTooLarge
		}
		return nil, "", "", fmt.Errorf("%w: read", ErrDownloadFailed)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", "", ErrPayloadTooLarge
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("%w: empty audio", ErrInvalidRequest)
	}
	filename := safeFilename(pathBaseFromURL(resp.Request.URL))
	if filename == "" {
		filename = "audio"
	}
	return data, detectAudioMIME(data, resp.Header.Get("Content-Type")), filename, nil
}

func pathBaseFromURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	base := filepath.Base(value.Path)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func ParseMultipartAudioOptions(delayRaw string, presenceRaw string, quotedIDRaw string, quotedRaw string, mentionAllRaw string) (*MessageOptions, error) {
	options := &MessageOptions{}
	hasValue := false
	if strings.TrimSpace(delayRaw) != "" {
		delay, err := strconv.ParseInt(strings.TrimSpace(delayRaw), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: delay must be integer", ErrInvalidRequest)
		}
		options.Delay = &delay
		hasValue = true
	}
	if strings.TrimSpace(presenceRaw) != "" {
		presence := strings.TrimSpace(presenceRaw)
		options.Presence = &presence
		hasValue = true
	}
	if strings.TrimSpace(quotedIDRaw) != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(quotedIDRaw), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: quotedMessageId must be integer", ErrInvalidRequest)
		}
		options.QuotedMessageID = &id
		hasValue = true
	}
	if strings.TrimSpace(quotedRaw) != "" {
		var quoted map[string]any
		if err := json.Unmarshal([]byte(quotedRaw), &quoted); err != nil {
			return nil, fmt.Errorf("%w: quotedMessage must be object", ErrQuotedMessageInvalid)
		}
		options.QuotedMessage = quoted
		hasValue = true
	}
	if strings.TrimSpace(mentionAllRaw) != "" {
		switch strings.ToLower(strings.TrimSpace(mentionAllRaw)) {
		case "true":
			value := true
			options.MentionAll = &value
		case "false":
			value := false
			options.MentionAll = &value
		default:
			return nil, fmt.Errorf("%w: mentionAll must be boolean", ErrInvalidRequest)
		}
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}
	return options, nil
}

func extensionFromMIME(mimeType string) string {
	exts, _ := mime.ExtensionsByType(strings.Split(mimeType, ";")[0])
	if len(exts) == 0 {
		return ""
	}
	return exts[0]
}
