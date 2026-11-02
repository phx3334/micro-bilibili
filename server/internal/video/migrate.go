package video

import (
	"fake_tiktok/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AutoMigrateVideo 只迁移视频库（feed_video）的表：videos。
func AutoMigrateVideo(db *gorm.DB, lg *zap.Logger) error {
	if err := db.AutoMigrate(&model.Video{}); err != nil {
		lg.Error("视频库迁移失败", zap.Error(err))
		return err
	}

	lg.Info("视频库迁移完成", zap.String("database", "feed_video"))
	return nil
}
