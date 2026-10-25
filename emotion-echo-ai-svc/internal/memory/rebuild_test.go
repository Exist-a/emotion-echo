// CL-08d：重建编排的单元测试（确定性替身）。
package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWriterForRebuild struct {
	upserts   []Episode
	uuids     []string
	upsertErr error
	resetErr  error
	resetFor  []int64
	removed   []int64
}

func (f *fakeWriterForRebuild) UpsertEpisode(ctx context.Context, ep Episode) (string, error) {
	if f.upsertErr != nil {
		return "", f.upsertErr
	}
	f.upserts = append(f.upserts, ep)
	f.uuids = append(f.uuids, "u"+string(rune('a'+len(f.uuids))))
	return f.uuids[len(f.uuids)-1], nil
}

func (f *fakeWriterForRebuild) DeleteUserData(ctx context.Context, userID int64) error {
	if f.resetErr != nil {
		return f.resetErr
	}
	f.resetFor = append(f.resetFor, userID)
	return nil
}

func (f *fakeWriterForRebuild) RemoveEpisode(ctx context.Context, userID int64, uuid string) error {
	f.removed = append(f.removed, userID)
	return nil
}

type fakeUUIDSetter struct {
	uuids map[string]string // "kind:id" -> uuid
}

func (f *fakeUUIDSetter) SetEpisodeGraphUUID(ctx context.Context, kind string, id int64, uuid string) error {
	f.uuids[kind+":"+itoa(id)] = uuid
	return nil
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

func TestRebuildUserGraph_ResetsThenReplaysInOrder(t *testing.T) {
	w := &fakeWriterForRebuild{}
	setter := &fakeUUIDSetter{uuids: map[string]string{}}
	eps := []EpisodeSource{
		{SourceKind: "message", SourceID: 2, UserID: 7, Name: "b", Content: "第二", ValidAtUnixMs: 2000},
		{SourceKind: "message", SourceID: 1, UserID: 7, Name: "a", Content: "第一", ValidAtUnixMs: 1000},
		{SourceKind: "note", SourceID: 3, UserID: 7, Name: "c", Content: "", ValidAtUnixMs: 3000}, // 空 content 跳过
	}
	ok, failed, err := RebuildUserGraph(context.Background(), w, setter, eps, 7)
	require.NoError(t, err)
	assert.Equal(t, 2, ok)
	assert.Equal(t, 0, failed)
	assert.Equal(t, []int64{7}, w.resetFor, "重放前必须先清图")
	require.Len(t, w.upserts, 2)
	assert.Equal(t, "第二", w.upserts[0].Content, "按入参顺序重放（排序责任在调用方/SQL ORDER BY）")
	assert.Equal(t, "ua", setter.uuids["message:2"], "uuid 必须回填 PG")
	assert.Equal(t, "ub", setter.uuids["message:1"])
}

func TestRebuildUserGraph_ResetFailureAborts(t *testing.T) {
	w := &fakeWriterForRebuild{resetErr: errors.New("graph down")}
	_, _, err := RebuildUserGraph(context.Background(), w, nil, nil, 7)
	require.Error(t, err)
	assert.Len(t, w.upserts, 0, "清图失败不得重放")
}

func TestRebuildUserGraph_PerEpisodeFailureContinues(t *testing.T) {
	w := &fakeWriterForRebuild{upsertErr: errors.New("llm down")}
	ok, failed, err := RebuildUserGraph(context.Background(), w, nil, []EpisodeSource{
		{SourceKind: "message", SourceID: 1, UserID: 7, Name: "a", Content: "x", ValidAtUnixMs: 1},
	}, 7)
	require.NoError(t, err)
	assert.Equal(t, 0, ok)
	assert.Equal(t, 1, failed, "单条失败不中断重建")
}
