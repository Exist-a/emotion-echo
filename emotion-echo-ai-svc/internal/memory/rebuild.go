// Package memory — rebuild.go
//
// CL-08d：「从 PG 重建图」工具的核心编排（ADR 决策 8 的配套——图丢了/换了后端/
// 迁移残留都能从 PG 重放）。供两处消费：
//   - cmd/memory-rebuild（运维一次性命令）
//   - DeleteMemory 的 uuid 缺失回落路径（单条删除找不到 graph_uuid 时整图重建）
//
// 语义：以 PG 为准 —— 只重放**未软删**的情节（valid_at ASC 保证同会话时序）；
// 重放会重新生成 episode uuid ⇒ 逐条回填 PG（删改后重建的闭环完整性）。
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// EpisodeUUIDSetter 重建后回填 uuid 的仓储能力（MemoryFactRepo 的窄视图）。
type EpisodeUUIDSetter interface {
	SetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64, uuid string) error
}

// EpisodeSource 重建的情节来源（PG 行的窄视图）。
type EpisodeSource struct {
	SourceKind    string
	SourceID      int64
	UserID        int64
	Name          string
	Content       string
	ValidAtUnixMs int64
}

// RebuildUserGraph 重放一个用户的全部未删情节。
// 返回 (重放成功数, 失败数, 错误)。单条失败不中断（记日志继续）——
// 重建本身是修复动作，宁可部分成功也要把能重建的重建出来。
func RebuildUserGraph(ctx context.Context, writer EpisodeGraphWriter, setter EpisodeUUIDSetter,
	episodes []EpisodeSource, userID int64) (ok, failed int, err error) {

	// 0) 先清空该用户的图（物理分图快路径）——重建 = 从零重放
	if err := writer.DeleteUserData(ctx, userID); err != nil {
		return 0, 0, fmt.Errorf("reset user graph: %w", err)
	}
	for _, ep := range episodes {
		if ep.Content == "" {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, graphRebuildEpisodeTimeout)
		uuid, uerr := writer.UpsertEpisode(cctx, Episode{
			UserID:        ep.UserID,
			Name:          ep.Name,
			Content:       ep.Content,
			ValidAtUnixMs: ep.ValidAtUnixMs,
			SourceKind:    ep.SourceKind,
			SourceID:      ep.SourceID,
		})
		cancel()
		if uerr != nil {
			failed++
			slog.WarnContext(ctx, "memory rebuild: episode failed",
				"user_id", ep.UserID, "source_kind", ep.SourceKind, "source_id", ep.SourceID, "err", uerr)
			continue
		}
		if uuid != "" && setter != nil {
			if serr := setter.SetEpisodeGraphUUID(ctx, ep.SourceKind, ep.SourceID, uuid); serr != nil {
				slog.WarnContext(ctx, "memory rebuild: persist uuid failed",
					"source_kind", ep.SourceKind, "source_id", ep.SourceID, "err", serr)
			}
		}
		ok++
	}
	return ok, failed, nil
}

// graphRebuildEpisodeTimeout 单情节重放上限（摄取管线秒级；留重试余量）。
const graphRebuildEpisodeTimeout = 70 * time.Second
