package scanner

import (
	"errors"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"github.com/seanime-app/seanime-server/internal/anilist"
	"github.com/seanime-app/seanime-server/internal/anizip"
	"github.com/seanime-app/seanime-server/internal/comparison"
	"github.com/seanime-app/seanime-server/internal/limiter"
	"github.com/seanime-app/seanime-server/internal/util"
	"github.com/sourcegraph/conc/pool"
	"strconv"
	"time"
)

type FileHydrator struct {
	localFiles         []*LocalFile
	media              []*anilist.BaseMedia
	baseMediaCache     *anilist.BaseMediaCache
	anizipCache        *anizip.Cache
	anilistClient      *anilist.Client
	anilistRateLimiter *limiter.Limiter
}

// HydrateMetadata will hydrate the metadata of each LocalFile with the metadata of the matched anilist.BaseMedia.
func (fh *FileHydrator) HydrateMetadata() {
	rateLimiter := limiter.NewLimiter(5*time.Second, 20)

	// Group local files by media ID
	groups := lop.GroupBy(fh.localFiles, func(localFile *LocalFile) int {
		return localFile.MediaId
	})

	// Remove the group with unmatched media
	delete(groups, 0)

	// Process each group in parallel
	p := pool.New()
	for mId, files := range groups {
		mId := mId
		files := files
		p.Go(func() {
			if len(files) > 0 {
				fh.hydrateGroupMetadata(mId, files, rateLimiter)
			}
		})
	}
	p.Wait()
}

func (fh *FileHydrator) hydrateGroupMetadata(
	mId int,
	lfs []*LocalFile,
	rateLimiter *limiter.Limiter,
) {

	// Get the media
	media, found := lo.Find(fh.media, func(media *anilist.BaseMedia) bool {
		return media.ID == mId
	})
	if !found {
		return
	}

	// Tree contains media relations
	tree := anilist.NewBaseMediaRelationTree()
	// Tree analysis used for episode normalization
	var mediaTreeAnalysis *MediaTreeAnalysis
	treeFetched := false

	// Process each local file in the group sequentially
	lo.ForEach(lfs, func(lf *LocalFile, index int) {

		// Get episode number
		episode := -1
		if len(lf.ParsedData.Episode) > 0 {
			if ep, ok := util.StringToInt(lf.ParsedData.Episode); ok {
				episode = ep
			}
		}

		if comparison.ValueContainsSpecial(lf.Name) {
			lf.Metadata.IsSpecial = true
			if episode > -1 {
				lf.Metadata.Episode = episode
				lf.Metadata.AniDBEpisode = "S" + strconv.Itoa(episode)
			}
			return
		}
		if comparison.ValueContainsNC(lf.Name) {
			lf.Metadata.Episode = 1
			lf.Metadata.AniDBEpisode = "NC"
			lf.Metadata.IsNC = true
			return
		}
		// Movie metadata
		if *media.Format == anilist.MediaFormatMovie {
			lf.Metadata.Episode = 1
			lf.Metadata.AniDBEpisode = "1"
			return
		}

		// No absolute episode count
		if episode <= media.GetCurrentEpisodeCount() {
			// Episode 0 - Might be a special
			// By default, we will assume that AniDB doesn't include it as main episodes (which is often the case)
			if episode == 0 {
				// Leave episode number as 0, assuming that the client will handle tracking correctly
				lf.Metadata.Episode = 0
				lf.Metadata.AniDBEpisode = "S1"
				return
			}

			lf.Metadata.Episode = episode
			lf.Metadata.AniDBEpisode = strconv.Itoa(episode)
			return
		}

		// Episode number is higher but media only has 1 episode
		// - Might be a movie that was not correctly identified as such
		// - Or, the torrent files were divided into multiple episodes from a media that is listed as a movie on AniList
		if episode > media.GetCurrentEpisodeCount() && media.GetTotalEpisodeCount() == 1 {
			lf.Metadata.Episode = 1 // Coerce episode number to 1 because it is used for tracking
			lf.Metadata.AniDBEpisode = "1"
			return
		}

		// Absolute episode count
		if episode > media.GetCurrentEpisodeCount() {
			// Fetch media tree
			if !treeFetched {
				if err := media.FetchMediaTree(anilist.FetchMediaTreeAll, fh.anilistClient, fh.anilistRateLimiter, tree, fh.baseMediaCache); err == nil {
					// Create a new media tree analysis that will be used for episode normalization
					mta := NewMediaTreeAnalysis(&MediaTreeAnalysisOptions{
						tree:        tree,
						anizipCache: fh.anizipCache,
						rateLimiter: rateLimiter,
					})
					// Hoist the media tree analysis, so it will be used by other files
					mediaTreeAnalysis = mta
					treeFetched = true

					if err := fh.normalizeEpisodeNumber(mediaTreeAnalysis, lf, episode); err != nil {
						println("an error occurred while normalizing episode number", err.Error())
					}
				}
			} else {
				if err := fh.normalizeEpisodeNumber(mediaTreeAnalysis, lf, episode); err != nil {
					println("an error occurred while normalizing episode number", err.Error())
				}
			}
			return
		}

	})

}

func (fh *FileHydrator) normalizeEpisodeNumber(
	mta *MediaTreeAnalysis,
	lf *LocalFile,
	ep int,
) error {
	if mta == nil {
		lf.Metadata.Episode = ep
		lf.Metadata.AniDBEpisode = strconv.Itoa(ep)
		return errors.New("[hydrator] could not find media tree analysis")
	}

	relativeEp, ok := mta.getRelativeEpisodeNumber(ep)
	if !ok {
		lf.Metadata.Episode = ep
		lf.Metadata.AniDBEpisode = strconv.Itoa(ep)
		return errors.New("[hydrator] could not normalize episode number")
	}

	lf.Metadata.Episode = relativeEp
	lf.Metadata.AniDBEpisode = strconv.Itoa(relativeEp)
	return nil
}
