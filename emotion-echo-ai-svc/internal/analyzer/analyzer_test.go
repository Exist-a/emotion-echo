package analyzer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeywordAnalyzer_HappyText(t *testing.T) {
	t.Parallel()
	a := NewKeywordAnalyzer()
	got, err := a.Analyze(context.Background(), "今天很开心，运气真好，谢谢你")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Greater(t, got.SentimentScore, 0.0, "正面文本 sentiment 应 > 0")
	assert.Equal(t, "keyword-stub-v1", got.Model)
}

func TestKeywordAnalyzer_SadText(t *testing.T) {
	t.Parallel()
	a := NewKeywordAnalyzer()
	got, err := a.Analyze(context.Background(), "我今天很难过，糟糕透了")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sad", got.PrimaryEmotion)
	assert.Less(t, got.SentimentScore, 0.0, "负面文本 sentiment 应 < 0")
}

func TestKeywordAnalyzer_EmptyText_Neutral(t *testing.T) {
	t.Parallel()
	a := NewKeywordAnalyzer()
	got, err := a.Analyze(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "neutral", got.PrimaryEmotion)
	assert.Equal(t, 0.0, got.SentimentScore)
}

func TestKeywordAnalyzer_NeutralText(t *testing.T) {
	t.Parallel()
	a := NewKeywordAnalyzer()
	got, err := a.Analyze(context.Background(), "我去吃饭了")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "neutral", got.PrimaryEmotion)
}