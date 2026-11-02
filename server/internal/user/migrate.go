package user

import (
	"fake_tiktok/internal/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AutoMigrateUser 只迁移用户库（feed_user）的表：accounts、user_follows。
//
// 拆库边界：用户库只保存账户与关注关系，不保存任何视频维度统计。
func AutoMigrateUser(db *gorm.DB, lg *zap.Logger) error {
	if err := db.AutoMigrate(
		&model.Account{},
		&model.UserFollow{},
	); err != nil {
		lg.Error("用户库迁移失败", zap.Error(err))
		return err
	}
	lg.Info("用户库迁移完成", zap.String("database", "feed_user"))
	return nil
}
