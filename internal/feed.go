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
