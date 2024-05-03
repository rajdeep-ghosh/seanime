package mediastream

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/seanime-app/seanime/internal/events"
	"github.com/seanime-app/seanime/internal/mediastream/transcoder"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ServeFiberTranscodeStream serves the transcoded segments
func (r *Repository) ServeFiberTranscodeStream(fiberCtx *fiber.Ctx, clientId string) error {

	if !r.IsInitialized() {
		r.wsEventManager.SendEvent(events.MediastreamShutdownStream, "Transcoding module not initialized")
		return errors.New("transcoding module not initialized")
	}

	if !r.TranscoderIsInitialized() {
		r.wsEventManager.SendEvent(events.MediastreamShutdownStream, "Transcoder not initialized")
		return errors.New("transcoder not initialized")
	}

	// Get the route parameters
	params := fiberCtx.AllParams()
	if len(params) == 0 {
		return errors.New("no params")
	}

	// Get the parameter group
	path := params["*1"]

	// Get current media
	mediaContainer, found := r.playbackManager.currentMediaContainer.Get()
	if !found {
		//
		// When the media container is not found but this route is called, something went wrong
		//
		r.wsEventManager.SendEvent(events.MediastreamShutdownStream, "No media has been requested")
		return errors.New("no media has been requested")
	}

	r.logger.Trace().Any("path", path).Msg("mediastream: Req")

	// /master.m3u8
	if path == "master.m3u8" {
		ret, err := r.transcoder.MustGet().GetMaster(mediaContainer.Filepath, mediaContainer.Hash, mediaContainer.MediaInfo, clientId)
		if err != nil {
			return err
		}

		time.Sleep(1 * time.Second)

		return fiberCtx.SendString(ret)
	}

	// Video stream
	// /:quality/index.m3u8
	if strings.HasSuffix(path, "index.m3u8") && !strings.Contains(path, "audio") {
		split := strings.Split(path, "/")
		if len(split) != 2 {
			return errors.New("invalid index.m3u8 path")
		}

		quality, err := transcoder.QualityFromString(split[0])
		if err != nil {
			return err
		}

		ret, err := r.transcoder.MustGet().GetVideoIndex(mediaContainer.Filepath, mediaContainer.Hash, mediaContainer.MediaInfo, quality, clientId)
		if err != nil {
			return err
		}

		return fiberCtx.SendString(ret)
	}

	// Audio stream
	// /audio/:audio/index.m3u8
	if strings.HasSuffix(path, "index.m3u8") && strings.Contains(path, "audio") {
		split := strings.Split(path, "/")
		if len(split) != 3 {
			return errors.New("invalid index.m3u8 path")
		}

		audio, err := strconv.ParseInt(split[1], 10, 32)
		if err != nil {
			return err
		}

		ret, err := r.transcoder.MustGet().GetAudioIndex(mediaContainer.Filepath, mediaContainer.Hash, mediaContainer.MediaInfo, int32(audio), clientId)
		if err != nil {
			return err
		}

		return fiberCtx.SendString(ret)
	}

	// Video segment
	// /:quality/segments-:chunk.ts
	if strings.HasSuffix(path, ".ts") && !strings.Contains(path, "audio") {
		split := strings.Split(path, "/")
		if len(split) != 2 {
			return errors.New("invalid segments-:chunk.ts path")
		}

		quality, err := transcoder.QualityFromString(split[0])
		if err != nil {
			return err
		}

		segment, err := transcoder.ParseSegment(split[1])
		if err != nil {
			return err
		}

		ret, err := r.transcoder.MustGet().GetVideoSegment(mediaContainer.Filepath, mediaContainer.Hash, mediaContainer.MediaInfo, quality, segment, clientId)
		if err != nil {
			return err
		}

		return fiberCtx.SendFile(ret)
	}

	// Audio segment
	// /audio/:audio/segments-:chunk.ts
	if strings.HasSuffix(path, ".ts") && strings.Contains(path, "audio") {
		split := strings.Split(path, "/")
		if len(split) != 3 {
			return errors.New("invalid segments-:chunk.ts path")
		}

		audio, err := strconv.ParseInt(split[1], 10, 32)
		if err != nil {
			return err
		}

		segment, err := transcoder.ParseSegment(split[2])
		if err != nil {
			return err
		}

		ret, err := r.transcoder.MustGet().GetAudioSegment(mediaContainer.Filepath, mediaContainer.Hash, mediaContainer.MediaInfo, int32(audio), segment, clientId)
		if err != nil {
			return err
		}

		return fiberCtx.SendFile(ret)
	}

	return errors.New("invalid path")
}

// ServeFiberTranscodeSubtitles serves the extracted subtitles
func (r *Repository) ServeFiberTranscodeSubtitles(fiberCtx *fiber.Ctx) error {

	if !r.IsInitialized() {
		r.wsEventManager.SendEvent(events.MediastreamShutdownStream, "Transcoding module not initialized")
		return errors.New("transcoding module not initialized")
	}

	if !r.TranscoderIsInitialized() {
		r.wsEventManager.SendEvent(events.MediastreamShutdownStream, "Transcoder not initialized")
		return errors.New("transcoder not initialized")
	}

	// Get the route parameters
	params := fiberCtx.AllParams()
	if len(params) == 0 {
		return errors.New("no params")
	}

	// Get the parameter group
	subFilePath := params["*1"]

	// Get current media
	mediaContainer, found := r.playbackManager.currentMediaContainer.Get()
	if !found {
		return errors.New("no media has been requested")
	}

	r.logger.Trace().Any("subFilePath", subFilePath).Msg("mediastream: Req")

	retPath := ""
	switch mediaContainer.StreamType {
	case StreamTypeTranscode:
		retPath = filepath.Join(r.transcoder.MustGet().GetSettings().MetadataDir, mediaContainer.Hash, "sub", subFilePath)
	default:
		// TODO: Implement for other stream types
	}

	if retPath == "" {
		return errors.New("could not find subtitles")
	}

	return fiberCtx.SendFile(retPath)
}

// ShutdownTranscodeStream It should be called when unmounting the player (playback is no longer needed).
// This will also send an events.MediastreamShutdownStream event.
func (r *Repository) ShutdownTranscodeStream() {

	if !r.IsInitialized() {
		return
	}

	if !r.TranscoderIsInitialized() {
		return
	}

	if !r.playbackManager.currentMediaContainer.IsPresent() {
		return
	}

	// Kill playback
	r.playbackManager.KillPlayback()

	// Destroy the current transcoder
	r.transcoder.MustGet().Destroy()

	// Load a new transcoder
	//r.transcoder = mo.None[*transcoder.Transcoder]()
	//r.initializeTranscoder(r.settings)

	// Send event
	r.wsEventManager.SendEvent(events.MediastreamShutdownStream, nil)
}
