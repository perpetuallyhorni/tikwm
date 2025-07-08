package tikwm

// AssetType defines the type of media asset.
type AssetType string

const (
	// AssetHD represents a high-definition asset.
	AssetHD AssetType = "hd"
	// AssetSD represents a standard-definition asset.
	AssetSD AssetType = "sd"
	// AssetSource represents the original source asset.
	AssetSource AssetType = "source"
	// AssetCoverMedium represents a medium-sized cover asset.
	AssetCoverMedium AssetType = "cover_medium"
	// AssetCoverOrigin represents the original cover asset.
	AssetCoverOrigin AssetType = "cover_origin"
	// AssetCoverDynamic represents a dynamically generated cover asset.
	AssetCoverDynamic AssetType = "cover_dynamic"
	// AssetAlbumPhoto represents an album photo asset.
	AssetAlbumPhoto AssetType = "album_photo"
	// AssetAvatar represents an avatar asset.
	AssetAvatar AssetType = "avatar"
	// AssetCover represents a generic cover asset for DB operations.
	AssetCover AssetType = "cover" // Generic type for DB operations
)

// Post represents a TikTok post.
type Post struct {
	// Id is the unique identifier of the post.
	Id string `json:"id"`
	// VideoId is the unique identifier of the video.
	VideoId string `json:"video_id"`
	// Region is the region where the post was created.
	Region string `json:"region"`
	// Title is the title of the post.
	Title string `json:"title"`
	// Cover is the URL of the post's cover image.
	Cover string `json:"cover"`
	// OriginCover is the URL of the original cover image.
	OriginCover string `json:"origin_cover"`
	// AiDynamicCover is the URL of the AI-generated dynamic cover image.
	AiDynamicCover string `json:"ai_dynamic_cover"`
	// Duration is the duration of the video in seconds.
	Duration int `json:"duration"`
	// Play is the URL of the video.
	Play string `json:"play"`
	// Wmplay is the URL of the watermarked video.
	Wmplay string `json:"wmplay"`
	// Hdplay is the URL of the high-definition video.
	Hdplay string `json:"hdplay"`
	// Size is the size of the video in bytes.
	Size int `json:"size"`
	// WmSize is the size of the watermarked video in bytes.
	WmSize int `json:"wm_size"`
	// HdSize is the size of the high-definition video in bytes.
	HdSize int `json:"hd_size"`
	// Music is the URL of the music used in the post.
	Music string `json:"music"`
	// MusicInfo contains information about the music used in the post.
	MusicInfo struct {
		// Id is the unique identifier of the music.
		Id string `json:"id"`
		// Title is the title of the music.
		Title string `json:"title"`
		// Play is the URL of the music.
		Play string `json:"play"`
		// Cover is the URL of the music's cover image.
		Cover string `json:"cover"`
		// Author is the author of the music.
		Author string `json:"author"`
		// Original indicates whether the music is original.
		Original bool `json:"original"`
		// Duration is the duration of the music, can be a number or a string.
		Duration interface{} `json:"duration"` // Changed to interface{} to handle number or string
		// Album is the album the music belongs to.
		Album string `json:"album"`
	} `json:"music_info"`
	// PlayCount is the number of times the post has been played.
	PlayCount int `json:"play_count"`
	// DiggCount is the number of likes the post has received.
	DiggCount int `json:"digg_count"`
	// CommentCount is the number of comments the post has received.
	CommentCount int `json:"comment_count"`
	// ShareCount is the number of times the post has been shared.
	ShareCount int `json:"share_count"`
	// DownloadCount is the number of times the post has been downloaded.
	DownloadCount int `json:"download_count"`
	// CollectCount is the number of times the post has been collected.
	CollectCount int `json:"collect_count"`
	// CreateTime is the timestamp of when the post was created.
	CreateTime int64 `json:"create_time"`
	// Anchors is a list of anchors in the post.
	Anchors interface{} `json:"anchors"`
	// AnchorsExtras is extra information about the anchors.
	AnchorsExtras string `json:"anchors_extras"`
	// IsAd indicates whether the post is an advertisement.
	IsAd bool `json:"is_ad"`
	// CommerceInfo contains information about the post's commercial aspects.
	CommerceInfo struct {
		// AdvPromotable indicates whether the post can be promoted as an advertisement.
		AdvPromotable bool `json:"adv_promotable"`
		// AuctionAdInvited indicates whether the post has been invited to an auction ad.
		AuctionAdInvited bool `json:"auction_ad_invited"`
		// BrandedContentType is the type of branded content in the post.
		BrandedContentType int `json:"branded_content_type"`
		// WithCommentFilterWords indicates whether the post uses comment filter words.
		WithCommentFilterWords bool `json:"with_comment_filter_words"`
	} `json:"commerce_info"`
	// CommercialVideoInfo is information about the commercial video.
	CommercialVideoInfo string `json:"commercial_video_info"`
	// Author contains information about the author of the post.
	Author struct {
		// Id is the unique identifier of the author.
		Id string `json:"id"`
		// UniqueId is the unique ID of the author.
		UniqueId string `json:"unique_id"`
		// Nickname is the nickname of the author.
		Nickname string `json:"nickname"`
		// Avatar is the URL of the author's avatar image.
		Avatar string `json:"avatar"`
	} `json:"author"`
	// Images is a list of image URLs in the post.
	Images []string `json:"images"`
}

// ID returns the ID of the post, using VideoId if Id is empty.
func (post Post) ID() string {
	if post.Id != "" {
		return post.Id
	}
	return post.VideoId
}

// UserFeed represents a feed of videos for a user.
type UserFeed struct {
	// Videos is a list of videos in the feed.
	Videos []Post `json:"videos"`
	// Cursor is the cursor for pagination.
	Cursor string `json:"cursor"`
	// HasMore indicates whether there are more videos to load.
	HasMore bool `json:"hasMore"`
}

// UserDetail represents detailed information about a user.
type UserDetail struct {
	// User contains information about the user.
	User struct {
		// Id is the unique identifier of the user.
		Id string `json:"id"`
		// UniqueId is the unique ID of the user.
		UniqueId string `json:"uniqueId"`
		// Nickname is the nickname of the user.
		Nickname string `json:"nickname"`
		// AvatarThumb is the URL of the user's thumbnail avatar image.
		AvatarThumb string `json:"avatarThumb"`
		// AvatarMedium is the URL of the user's medium avatar image.
		AvatarMedium string `json:"avatarMedium"`
		// AvatarLarger is the URL of the user's larger avatar image.
		AvatarLarger string `json:"avatarLarger"`
		// Signature is the user's signature.
		Signature string `json:"signature"`
		// Verified indicates whether the user is verified.
		Verified bool `json:"verified"`
		// SecUid is the secure user ID.
		SecUid string `json:"secUid"`
		// Secret indicates whether the user is secret.
		Secret bool `json:"secret"`
		// Ftc indicates whether the user is FTC compliant.
		Ftc bool `json:"ftc"`
		// Relation is the relation of the current user to this user.
		Relation int `json:"relation"`
		// OpenFavorite indicates whether the user's favorites are open.
		OpenFavorite bool `json:"openFavorite"`
		// CommentSetting is the user's comment setting.
		CommentSetting interface{} `json:"commentSetting"`
		// DuetSetting is the user's duet setting.
		DuetSetting interface{} `json:"duetSetting"`
		// StitchSetting is the user's stitch setting.
		StitchSetting interface{} `json:"stitchSetting"`
		// PrivateAccount indicates whether the user's account is private.
		PrivateAccount bool `json:"privateAccount"`
		// IsADVirtual indicates whether the user is a virtual ad.
		IsADVirtual bool `json:"isADVirtual"`
		// IsUnderAge18 indicates whether the user is under 18.
		IsUnderAge18 bool `json:"isUnderAge18"`
		// InsId is the user's Instagram ID.
		InsId string `json:"ins_id"`
		// TwitterId is the user's Twitter ID.
		TwitterId string `json:"twitter_id"`
		// YoutubeChannelTitle is the user's YouTube channel title.
		YoutubeChannelTitle string `json:"youtube_channel_title"`
		// YoutubeChannelId is the user's YouTube channel ID.
		YoutubeChannelId string `json:"youtube_channel_id"`
	} `json:"user"`
	// Stats contains statistics about the user.
	Stats struct {
		// FollowingCount is the number of users the user is following.
		FollowingCount int `json:"followingCount"`
		// FollowerCount is the number of followers the user has.
		FollowerCount int `json:"followerCount"`
		// HeartCount is the number of hearts the user has received.
		HeartCount int `json:"heartCount"`
		// VideoCount is the number of videos the user has created.
		VideoCount int `json:"videoCount"`
		// DiggCount is the number of likes the user has received.
		DiggCount int `json:"diggCount"`
		// Heart is an alias for HeartCount.
		Heart int `json:"heart"`
	} `json:"stats"`
}
