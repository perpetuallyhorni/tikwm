package tikwm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/perpetuallyhorni/tikwm/pkg/ratelimiter"
)

var (
	// URL is the base URL for the tikwm API.
	URL string = "https://tikwm.com/api"
	// RequestDelay is the delay between API requests to avoid rate-limiting.
	RequestDelay time.Duration = 1250 * time.Millisecond
	// MaxUserFeedCount is the number of posts to fetch per user feed request.
	MaxUserFeedCount int = 34
	// Debug enables verbose logging of API responses.
	Debug = false

	// apiRateLimiter is the global rate limiter for all API requests.
	apiRateLimiter     *ratelimiter.RateLimiter
	initRateLimiterMux sync.Mutex
	rootCtx            context.Context
	cancelRootCtx      context.CancelFunc
)

// InitRateLimiter initializes the global API rate limiter.
// This must be called once at application startup.
func InitRateLimiter(ctx context.Context) {
	initRateLimiterMux.Lock()
	defer initRateLimiterMux.Unlock()
	if apiRateLimiter == nil {
		rootCtx, cancelRootCtx = context.WithCancel(ctx)
		apiRateLimiter = ratelimiter.New(RequestDelay, rootCtx)
	}
}

// StopRateLimiter stops the global API rate limiter.
// This must be called once at application shutdown.
func StopRateLimiter() {
	initRateLimiterMux.Lock()
	defer initRateLimiterMux.Unlock()
	if apiRateLimiter != nil {
		cancelRootCtx()
		apiRateLimiter.Stop()
		apiRateLimiter = nil
	}
}

// wait blocks until a permit is available from the global rate limiter.
func wait() error {
	initRateLimiterMux.Lock()
	defer initRateLimiterMux.Unlock()
	if apiRateLimiter == nil {
		return errors.New("rate limiter not initialized, call InitRateLimiter first")
	}
	return apiRateLimiter.Wait()
}

// SourceEncodeResult represents the final successful result from the source encode endpoint.
type SourceEncodeResult struct {
	PlayURL string `json:"play_url"` // PlayURL is the URL of the encoded video.
	Size    int    `json:"size"`     // Size is the size of the encoded video in bytes.
}

// Raw executes a raw request to the tikwm API.
func Raw(method string, query map[string]string) ([]byte, error) {
	if err := wait(); err != nil {
		return nil, fmt.Errorf("rate limiter stopped: %w", err)
	}

	urlPath := fmt.Sprintf("%s/%s", URL, method)              // Construct the full URL.
	req, err := http.NewRequest(http.MethodGet, urlPath, nil) // Create a new HTTP request.
	if err != nil {
		return nil, err // Return an error if the request could not be created.
	}
	q := req.URL.Query()          // Get the query parameters.
	for key, val := range query { // Iterate over the query parameters.
		q.Add(key, val) // Add the query parameter to the URL.
	}
	req.URL.RawQuery = q.Encode()           // Encode the query parameters.
	resp, err := http.DefaultClient.Do(req) // Execute the HTTP request.
	if err != nil {
		return nil, err // Return an error if the request failed.
	}
	defer func() {
		if err := resp.Body.Close(); err != nil { // Close the response body.
			log.Printf("error closing response body: %v", err) // Log any errors that occur while closing the response body.
		}
	}()
	buffer, err := io.ReadAll(resp.Body) // Read the response body.
	if err != nil {
		return nil, err // Return an error if the response body could not be read.
	}
	if Debug {
		log.Print(string(buffer)) // Log the response body if debugging is enabled.
	}
	return buffer, nil // Return the response body.
}

// RawParsed executes a raw request and parses the JSON response.
func RawParsed[T any](method string, query map[string]string) (*T, error) {
	data, err := Raw(method, query) // Execute the raw request.
	if err != nil {
		return nil, err // Return an error if the request failed.
	}
	var resp struct {
		Code          int     `json:"code"`           // Code is the response code.
		Msg           string  `json:"msg"`            // Msg is the response message.
		ProcessedTime float64 `json:"processed_time"` // ProcessedTime is the time it took to process the request.
		Data          *T      `json:"data"`           // Data is the response data.
	}
	if err := json.Unmarshal(data, &resp); err != nil { // Unmarshal the response body.
		// Attempt to unmarshal just the error part for better diagnostics on malformed success responses
		var errResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Code != 0 {
			return nil, fmt.Errorf("tikwm error: %s (%d) [%s]", errResp.Msg, errResp.Code, method)
		}
		return nil, fmt.Errorf("failed to unmarshal tikwm response: %w. raw: %s", err, string(data))
	}
	if resp.Code != 0 { // Check if the response code is not 0.
		queryStr := "???"                                // Default query string.
		if buf, err := json.Marshal(query); err == nil { // Marshal the query parameters.
			queryStr = string(buf) // Convert the query parameters to a string.
		}
		return nil, fmt.Errorf("tikwm error: %s (%d) [%s, query: %s]", resp.Msg, resp.Code, method, queryStr) // Return an error if the response code is not 0.
	}
	return resp.Data, nil // Return the response data.
}

// submitSourceEncodeTask submits a video for source encoding and returns a task ID.
func submitSourceEncodeTask(videoID string) (string, error) {
	if err := wait(); err != nil {
		return "", fmt.Errorf("rate limiter stopped: %w", err)
	}

	var resp struct {
		TaskID string `json:"task_id"` // TaskID is the ID of the source encoding task.
	}
	// This is a POST request, so we can't use RawParsed.
	urlPath := fmt.Sprintf("%s/video/task/submit", URL) // Construct the full URL.
	formData := make(url.Values)                        // Initialize the map
	formData.Set("web", "1")                            // Set the web parameter.
	formData.Set("url", videoID)                        // Set the URL parameter.

	// Execute the HTTP request.
	httpResp, err := http.PostForm(urlPath, formData) // #nosec G107
	if err != nil {
		return "", err // Return an error if the request failed.
	}
	defer func() { _ = httpResp.Body.Close() }() // Close the response body.
	body, err := io.ReadAll(httpResp.Body)       // Read the response body.
	if err != nil {
		return "", err // Return an error if the response body could not be read.
	}

	var baseResp struct {
		Code int             `json:"code"` // Code is the response code.
		Msg  string          `json:"msg"`  // Msg is the response message.
		Data json.RawMessage `json:"data"` // Data is the response data.
	}
	if err := json.Unmarshal(body, &baseResp); err != nil { // Unmarshal the response body.
		return "", err // Return an error if the response body could not be unmarshaled.
	}
	if baseResp.Code != 0 { // Check if the response code is not 0.
		return "", fmt.Errorf("failed to submit task: %s (%d)", baseResp.Msg, baseResp.Code) // Return an error if the response code is not 0.
	}
	if err := json.Unmarshal(baseResp.Data, &resp); err != nil { // Unmarshal the response data.
		return "", err // Return an error if the response data could not be unmarshaled.
	}
	if resp.TaskID == "" { // Check if the task ID is empty.
		return "", errors.New("API returned an empty task ID") // Return an error if the task ID is empty.
	}
	return resp.TaskID, nil // Return the task ID.
}

// pollSourceEncodeResult polls the API for the result of a source encode task.
func pollSourceEncodeResult(taskID string) (*SourceEncodeResult, error) {
	var resp struct {
		Status int                 `json:"status"` // Status is the status of the source encoding task (2=success, 3=failure).
		Detail *SourceEncodeResult `json:"detail"` // Detail is the details of the source encoding result.
	}
	for i := 0; i < 60; i++ { // Poll for up to 60 seconds.
		// The polling loop itself calls RawParsed, which is rate-limited.
		data, err := RawParsed[json.RawMessage]("video/task/result", map[string]string{"task_id": taskID})
		if err != nil {
			if strings.Contains(err.Error(), "(-1)") { // Is it a rate limit error?
				time.Sleep(2 * time.Second) // Wait a bit longer if rate limited during polling
			}
			continue // Ignore transient errors and retry
		}
		if err := json.Unmarshal(*data, &resp); err != nil { // Unmarshal the response data.
			continue
		}
		switch resp.Status {
		case 2: // Success
			return resp.Detail, nil // Return the source encoding result.
		case 3: // Failure
			return nil, errors.New("source encode task failed or no higher quality available") // Return an error if the source encoding task failed.
		}
		// Status is still pending, continue polling.
		// A small sleep is good practice to not hammer the API, even with rate limiting.
		time.Sleep(1 * time.Second)
	}
	return nil, errors.New("source encode task timed out") // Return an error if the source encoding task timed out.
}

// GetSourceEncode gets the highest quality "source" video link.
func GetSourceEncode(videoID string) (*SourceEncodeResult, error) {
	taskID, err := submitSourceEncodeTask(videoID) // Submit the source encoding task.
	if err != nil {
		return nil, fmt.Errorf("failed to submit source encode task: %w", err) // Return an error if the source encoding task could not be submitted.
	}
	return pollSourceEncodeResult(taskID) // Poll for the source encoding result.
}

// GetPost fetches a single post by URL or ID.
func GetPost(url string, hd ...bool) (*Post, error) {
	query := map[string]string{"url": url} // Construct the query parameters.
	if len(hd) == 0 || hd[0] {             // Check if the hd parameter is set.
		query["hd"] = "1" // Set the hd parameter.
	}
	return RawParsed[Post]("", query) // Execute the raw request.
}

// GetUserFeedRaw fetches a raw page of a user's feed.
func GetUserFeedRaw(uniqueID string, count int, cursor string) (*UserFeed, error) {
	query := map[string]string{"unique_id": uniqueID, "count": strconv.Itoa(count), "cursor": cursor} // Construct the query parameters.
	return RawParsed[UserFeed]("user/posts", query)                                                   // Execute the raw request.
}

// GetUserDetail fetches details for a user profile.
func GetUserDetail(uniqueID string) (*UserDetail, error) {
	query := map[string]string{"unique_id": uniqueID} // Construct the query parameters.
	return RawParsed[UserDetail]("user/info", query)  // Execute the raw request.
}
