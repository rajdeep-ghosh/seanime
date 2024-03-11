package playbackmanager_test

import (
	"context"
	"github.com/seanime-app/seanime/internal/anilist"
	"github.com/seanime-app/seanime/internal/db"
	"github.com/seanime-app/seanime/internal/events"
	"github.com/seanime-app/seanime/internal/playbackmanager"
	"github.com/seanime-app/seanime/internal/test_utils"
	"github.com/seanime-app/seanime/internal/util"
	"testing"
)

func getPlaybackManager(t *testing.T) (*playbackmanager.PlaybackManager, anilist.ClientWrapperInterface, *anilist.AnimeCollection, error) {

	logger := util.NewLogger()

	wsEventManager := events.NewMockWSEventManager(logger)

	database, err := db.NewDatabase(test_utils.ConfigData.Path.DataDir, test_utils.ConfigData.Database.Name, logger)

	if err != nil {
		t.Fatalf("error while creating database, %v", err)
	}

	anilistClientWrapper := anilist.TestGetMockAnilistClientWrapper()

	anilistCollection, err := anilistClientWrapper.AnimeCollection(context.Background(), nil)
	if err != nil {
		return nil, nil, nil, err
	}

	return playbackmanager.New(&playbackmanager.NewProgressManagerOptions{
		Logger:               logger,
		WSEventManager:       wsEventManager,
		AnilistClientWrapper: anilistClientWrapper,
		Database:             database,
		AnilistCollection:    anilistCollection,
		RefreshAnilistCollectionFunc: func() {
			// Do nothing
		},
	}), anilistClientWrapper, anilistCollection, nil
}
