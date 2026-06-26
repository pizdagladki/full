// Package service tests for the doConvert pipeline (white-box, same package).
package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	repomocks "github.com/pizdagladki/full/services/media/internal/api/repository/mocks"
	svcmocks "github.com/pizdagladki/full/services/media/internal/api/service/mocks"
)

// testClipService creates a clipService with mock dependencies for white-box tests.
func testClipService(
	repo *repomocks.MockClipRepository,
	store *svcmocks.MockObjectStore,
	runner *svcmocks.MockFFmpegRunner,
) *clipService {
	return &clipService{
		repo:   repo,
		store:  store,
		runner: runner,
		cfg: ClipServiceConfig{
			MaxUploadBytes: 10 * 1024 * 1024,
			DownloadURLTTL: 15 * time.Minute,
		},
		logger: zap.NewNop(),
	}
}

// nopReadCloser wraps a byte slice in an io.ReadCloser for store.Get mocks.
type nopReadCloser struct {
	*bytes.Reader
}

func (nopReadCloser) Close() error { return nil }

func newReadCloser(data []byte) io.ReadCloser {
	return nopReadCloser{bytes.NewReader(data)}
}

func TestDoConvert_StoreGetFails_MarkedFailed(t *testing.T) {
	// criterion: 4 — store.Get failure → UpdateConversion called with failed, source clip untouched
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)
	runnerMock := svcmocks.NewMockFFmpegRunner(ctrl)

	clipID := int64(7)
	webmKey := "clips/42/uuid.webm"
	mp4Key := "clips/42/7.mp4"

	storeMock.EXPECT().Get(gomock.Any(), webmKey).Return(nil, errors.New("minio down"))
	repoMock.EXPECT().UpdateConversion(gomock.Any(), clipID, "", domain.ConversionStatusFailed).Return(nil)
	// runner.Convert and store.Put must NOT be called
	// (no EXPECT set for them; gomock will fail the test if they are)

	svc := testClipService(repoMock, storeMock, runnerMock)
	svc.doConvert(webmKey, mp4Key, clipID)
}

func TestDoConvert_RunnerFails_MarkedFailed(t *testing.T) {
	// criterion: 4 — ffmpeg failure (non-zero exit) → UpdateConversion called with failed, store.Put NOT called
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)
	runnerMock := svcmocks.NewMockFFmpegRunner(ctrl)

	clipID := int64(7)
	webmKey := "clips/42/uuid.webm"
	mp4Key := "clips/42/7.mp4"

	storeMock.EXPECT().Get(gomock.Any(), webmKey).Return(newReadCloser([]byte("fake-webm")), nil)
	runnerMock.EXPECT().Convert(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("ffmpeg: exit status 1: invalid data found when processing input"))
	repoMock.EXPECT().UpdateConversion(gomock.Any(), clipID, "", domain.ConversionStatusFailed).Return(nil)
	// store.Put must NOT be called

	svc := testClipService(repoMock, storeMock, runnerMock)
	svc.doConvert(webmKey, mp4Key, clipID)
}

func TestDoConvert_Success_MarksDone(t *testing.T) {
	// criterion: 1 — successful conversion → store.Put called with MP4 key, UpdateConversion called with done
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)
	runnerMock := svcmocks.NewMockFFmpegRunner(ctrl)

	clipID := int64(7)
	webmKey := "clips/42/uuid.webm"
	mp4Key := "clips/42/7.mp4"

	storeMock.EXPECT().Get(gomock.Any(), webmKey).Return(newReadCloser([]byte("fake-webm")), nil)
	runnerMock.EXPECT().Convert(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	storeMock.EXPECT().Put(gomock.Any(), mp4Key, gomock.Any(), gomock.Any(), domain.ContentTypeMP4).Return(nil)
	repoMock.EXPECT().UpdateConversion(gomock.Any(), clipID, mp4Key, domain.ConversionStatusDone).Return(nil)

	svc := testClipService(repoMock, storeMock, runnerMock)
	svc.doConvert(webmKey, mp4Key, clipID)
}

func TestDoConvert_UploadFails_MarkedFailed(t *testing.T) {
	// criterion: 4 — MP4 upload to store fails → UpdateConversion called with failed
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)
	runnerMock := svcmocks.NewMockFFmpegRunner(ctrl)

	clipID := int64(7)
	webmKey := "clips/42/uuid.webm"
	mp4Key := "clips/42/7.mp4"

	storeMock.EXPECT().Get(gomock.Any(), webmKey).Return(newReadCloser([]byte("fake-webm")), nil)
	runnerMock.EXPECT().Convert(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	storeMock.EXPECT().Put(gomock.Any(), mp4Key, gomock.Any(), gomock.Any(), domain.ContentTypeMP4).
		Return(errors.New("minio upload failed"))
	repoMock.EXPECT().UpdateConversion(gomock.Any(), clipID, "", domain.ConversionStatusFailed).Return(nil)

	svc := testClipService(repoMock, storeMock, runnerMock)
	svc.doConvert(webmKey, mp4Key, clipID)
}

func TestGetMP4URL_RepoError(t *testing.T) {
	// criterion: 3 — repo error propagated
	t.Parallel()

	ctrl := gomock.NewController(t)
	repoMock := repomocks.NewMockClipRepository(ctrl)
	storeMock := svcmocks.NewMockObjectStore(ctrl)
	runnerMock := svcmocks.NewMockFFmpegRunner(ctrl)

	repoMock.EXPECT().GetByID(gomock.Any(), int64(5)).Return(domain.Clip{}, errors.New("db error"))

	svc := testClipService(repoMock, storeMock, runnerMock)
	_, err := svc.GetMP4URL(context.Background(), int64(42), int64(5))

	if err == nil {
		t.Fatal("GetMP4URL() error = nil, want error")
	}
}
