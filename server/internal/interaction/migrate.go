package interaction

import (
	"fake_tiktok/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AutoMigrateInteraction 只迁移互动库（feed_interaction）的表：
// comments、comment_likes、video_likes。
func AutoMigrateInteraction(db *gorm.DB, lg *zap.Logger) error {
	if err := db.AutoMigrate(
		&model.Comment{},
		&model.CommentLike{},
		&model.VideoLike{},
	); err != nil {
		lg.Error("互动库迁移失败", zap.Error(err))
		return err
	}

	lg.Info("互动库迁移完成", zap.String("database", "feed_interaction"))
	return nil
}
