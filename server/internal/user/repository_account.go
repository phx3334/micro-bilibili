package user

import (
	"context"
	"time"

	"fake_tiktok/internal/model"

	"gorm.io/gorm"
)

// defaultDBTimeout 是 MySQL 操作的默认超时时间。
// 当调用方传入的 context 没有设置 deadline 时，使用此默认值。
const defaultDBTimeout = 5 * time.Second

// withTimeout 为 context 添加超时控制。
// 若 ctx 已有 deadline，则直接返回原 ctx（透传模式），
// 避免覆盖调用方设置的更短超时时间。
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultDBTimeout)
}

// AccountRepo 用户账户数据仓储（只访问本服务的 feed_user 库）。
type AccountRepo struct {
	db *gorm.DB
}

func NewAccountRepo(db *gorm.DB) *AccountRepo {
	return &AccountRepo{db: db}
}

// Create 创建用户账户记录。
func (r *AccountRepo) Create(ctx context.Context, account *model.Account) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Create(account).Error
}

// FindByUsername 根据用户名查询用户。
func (r *AccountRepo) FindByUsername(ctx context.Context, username string) (*model.Account, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var user model.Account
	if err := r.db.WithContext(ctx).Where("username=?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查询用户。
func (r *AccountRepo) FindByID(ctx context.Context, id string) (*model.Account, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var user model.Account
	if err := r.db.WithContext(ctx).Where("id=?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByIDs 根据 ID 列表批量查询用户。
func (r *AccountRepo) FindByIDs(ctx context.Context, ids []string) ([]*model.Account, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var users []*model.Account
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Save 保存用户（全字段更新或插入）。
func (r *AccountRepo) Save(ctx context.Context, user *model.Account) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Save(user).Error
}

// Updates 更新用户指定字段。
func (r *AccountRepo) Updates(ctx context.Context, id string, updates map[string]interface{}) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id=?", id).Updates(updates).Error
}

// UpdateColumn 更新用户单个字段（不触发 GORM 钩子）。
func (r *AccountRepo) UpdateColumn(ctx context.Context, id string, column string, value interface{}) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id=?", id).Update(column, value).Error
}
