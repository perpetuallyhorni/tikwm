package tikwm

import (
	"log"
	"time"
)

// Predicate is a function that takes a Post pointer and returns a boolean.
// It's used for filtering posts.
type Predicate func(post *Post) bool

// FilterVideo is a Predicate that returns true if the post is a video.
func FilterVideo(post *Post) bool {
	return post.IsVideo()
}

// FilterPhoto is a Predicate that returns true if the post is an album (photo).
func FilterPhoto(post *Post) bool {
	return post.IsAlbum()
}

// Ensure that FilterVideo and FilterPhoto implement the Predicate interface.
var _ Predicate = FilterVideo
var _ Predicate = FilterPhoto

// WhileAfter returns a Predicate that returns true if the post's creation time is after the given time.
func WhileAfter(t time.Time) Predicate {
	return func(post *Post) bool {
		createTime := time.Unix(post.CreateTime, 0)
		return createTime.After(t)
	}
}

// FeedOpt contains options for fetching user feed.
type FeedOpt struct {
	// Filter is a Predicate used to filter posts.  Only posts that pass the filter are returned.
	Filter Predicate
	// While is a Predicate used to determine when to stop fetching posts.
	// Fetching stops when this predicate returns false.
	While Predicate
	// OnError is a function that is called when an error occurs.
	OnError func(err error)
	// OnFeedProgress is a function that is called after each page of posts is fetched.
	// It provides the current count of posts that have been processed.
	OnFeedProgress func(count int)
	// ReturnChan is a channel to which fetched posts are sent.
	ReturnChan chan Post
	// SD is a boolean indicating whether to fetch standard definition videos.  (Currently unused)
	SD bool
}

// Defaults sets default values for the FeedOpt if they are not already set.
func (opt *FeedOpt) Defaults() *FeedOpt {
	if opt == nil {
		opt = &FeedOpt{}
	}
	if opt.Filter == nil {
		opt.Filter = func(vid *Post) bool { return true }
	}
	if opt.While == nil {
		opt.While = func(vid *Post) bool { return true }
	}
	if opt.OnError == nil {
		opt.OnError = func(err error) {
			log.Print(err)
		}
	}
	if opt.OnFeedProgress == nil {
		opt.OnFeedProgress = func(count int) {}
	}
	if opt.ReturnChan == nil {
		opt.ReturnChan = make(chan Post)
	}
	return opt
}

// GetUserFeedAwait fetches the user feed and returns all posts in a slice.
// It blocks until all posts have been fetched and sent to the channel.
func GetUserFeedAwait(uniqueID string, opts ...*FeedOpt) ([]Post, error) {
	var opt *FeedOpt = nil
	if len(opts) != 0 {
		opt = opts[0]
	}

	postChan, _, err := GetUserFeed(uniqueID, opt)
	if err != nil {
		return nil, err
	}
	ret := []Post{}
	for post := range postChan {
		ret = append(ret, post)
	}
	return ret, nil
}

// GetUserFeed fetches the user feed and returns a channel to which posts are sent.
// It also returns the initial count of posts and an error, if any.
// The channel is closed when all posts have been fetched and sent.
func GetUserFeed(uniqueID string, opts ...*FeedOpt) (chan Post, int, error) {
	var opt *FeedOpt = nil
	if len(opts) != 0 {
		opt = opts[0]
	}
	opt = opt.Defaults()
	posts, err := userFeedSinceInternal(uniqueID, "0", opt, 0)
	if err != nil {
		return nil, 0, err
	}

	go func() {
		defer close(opt.ReturnChan)

		// Reverse posts to process from oldest to newest.
		for i := 0; i < len(posts)/2; i++ {
			posts[i], posts[len(posts)-i-1] = posts[len(posts)-i-1], posts[i]
		}

		for _, post := range posts {
			opt.ReturnChan <- post
		}
	}()
	return opt.ReturnChan, len(posts), err
}

// userFeedSinceInternal is a recursive function that fetches user feed posts since a given cursor.
func userFeedSinceInternal(uniqueID string, cursor string, opt *FeedOpt, currentCount int) ([]Post, error) {
	feed, err := GetUserFeedRaw(uniqueID, MaxUserFeedCount, cursor)
	if err != nil {
		return nil, err
	}
	ret := []Post{}
	for _, vid := range feed.Videos {
		if !opt.While(&vid) {
			opt.OnFeedProgress(currentCount + len(ret))
			return ret, nil
		}
		if !opt.Filter(&vid) {
			continue
		}
		ret = append(ret, vid)
	}

	newTotal := currentCount + len(ret)
	opt.OnFeedProgress(newTotal)

	if !feed.HasMore {
		return ret, nil
	}
	deeperRet, err := userFeedSinceInternal(uniqueID, feed.Cursor, opt, newTotal)
	if err != nil {
		return ret, err
	}
	return append(ret, deeperRet...), nil
}
