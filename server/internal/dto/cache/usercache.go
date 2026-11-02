package cache

import "fake_tiktok/internal/model"

// UserCacheData 用户缓存组合数据（静态 + 动态），供 logic 层使用。
// Redis 存储层拆分为 user:static 和 user:dynamic 两个 Hash，
// GetUserCache 通过 Pipeline 合并两个 Hash 的结果返回此组合结构。
type UserCacheData struct {
	ID             string `json:"id"`
	AvatarURL      string `json:"avatar_url"`
	Signature      string `json:"signature"`
	Username       string `json:"username"`
	Birthday       string `json:"birthday"`
	Gender         string `json:"gender"`
	FansCount      int64  `json:"fans_count"`
	FollowingCount int64  `json:"following_count"`

	// StaticHit 标记静态区缓存是否命中，由 GetUserCache 设置。
	// 调用方可据此决定是否对 static 区做降级回源：
	//   - StaticHit=true：静态区正常，无需回源
	//   - StaticHit=false：静态区未命中（可能被 DeleteUserCache 删除或过期），
	//     需要单独回源 static 区（动态区计数由 HIncrBy 实时维护，不需要回源）
	StaticHit bool `json:"-"`
}

// UserStaticData 用户静态信息，存储在 user:static:{id} Hash
type UserStaticData struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Signature string `json:"signature"`
	Birthday  string `json:"birthday"`
	Gender    string `json:"gender"`
}

// UserDynamicData 用户动态计数，存储在 user:dynamic:{id} Hash。
// 视频数 / 播放量 / 获赞量不再缓存：这三个统计属于视频维度，
// 下一步拆库后由视频库按需聚合，无法在用户库内实时维护。
type UserDynamicData struct {
	FansCount      int64 `json:"fans_count"`
	FollowingCount int64 `json:"following_count"`
}

// AccountToUserCacheData 将 model.Account 转换为 UserStaticData 和 UserDynamicData
// 注意：Account 表没有 fans_count 和 following_count，这两个字段由 BackfillUserCache 从 follow 表回填
func AccountToUserCacheData(acc *model.Account) (*UserStaticData, *UserDynamicData) {
	static := &UserStaticData{
		ID:        acc.ID,
		Username:  acc.Username,
		AvatarURL: acc.AvatarURL,
		Signature: acc.Signature,
		Birthday:  acc.Birthday,
		Gender:    acc.Gender,
	}
	dynamic := &UserDynamicData{
		// FansCount 和 FollowingCount 由 BackfillUserCache 从 follow 表查询后设置
	}
	return static, dynamic
}

// MergeUserCacheData 合并静态和动态数据为组合结构
func MergeUserCacheData(static *UserStaticData, dynamic *UserDynamicData) *UserCacheData {
	if static == nil && dynamic == nil {
		return nil
	}
	data := &UserCacheData{}
	if static != nil {
		data.ID = static.ID
		data.Username = static.Username
		data.AvatarURL = static.AvatarURL
		data.Signature = static.Signature
		data.Birthday = static.Birthday
		data.Gender = static.Gender
	}
	if dynamic != nil {
		data.FansCount = dynamic.FansCount
		data.FollowingCount = dynamic.FollowingCount
	}
	return data
}
