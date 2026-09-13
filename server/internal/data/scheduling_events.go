package data

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
	"vgpu/internal/biz"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	schedulingEventPageSize   = 200
	schedulingEventMaxPages   = 3
	schedulingEventMaxItems   = 50
	schedulingEventMaxMessage = 4 * 1024
	schedulingEventCacheItems = 128
	schedulingEventCacheBytes = 8 * 1024 * 1024
	schedulingEventTimeout    = 3 * time.Second
	schedulingEventTTL        = 15 * time.Second
	schedulingEventErrorTTL   = 2 * time.Second
	schedulingEventPageBytes  = 2 * 1024 * 1024
)

var errSchedulingEventPageTooLarge = errors.New("scheduling event response exceeds the page byte limit")

// Its own rate limiter, so event reads cannot throttle the watches.
func newBoundedSchedulingEventClient(config *rest.Config) (*kubernetes.Clientset, error) {
	eventConfig := rest.CopyConfig(config)
	limiter := rate.NewLimiter(1, 2)
	eventConfig.Wrap(func(transport http.RoundTripper) http.RoundTripper {
		return schedulingEventTransport{RoundTripper: transport, limiter: limiter}
	})
	return kubernetes.NewForConfig(eventConfig)
}

type schedulingEventTransport struct {
	http.RoundTripper
	limiter *rate.Limiter
}

func (t schedulingEventTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// Count retried attempts too.
	if err := t.limiter.Wait(request.Context()); err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		return nil, context.DeadlineExceeded
	}
	response, err := t.RoundTripper.RoundTrip(request)
	if err == nil && response.Body != nil {
		response.Body = &schedulingEventBody{ReadCloser: response.Body, remaining: schedulingEventPageBytes}
	}
	return response, err
}

type schedulingEventBody struct {
	io.ReadCloser
	remaining int
}

func (r *schedulingEventBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.ReadCloser.Read(extra[:])
		if n > 0 {
			return 0, errSchedulingEventPageTooLarge
		}
		return 0, err
	}
	if len(buffer) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	n, err := r.ReadCloser.Read(buffer)
	r.remaining -= n
	return n, err
}

type schedulingEventEntry struct {
	value   biz.SchedulingEvents
	expires time.Time
	bytes   int
}

// Only events are cached; Pod status always comes from the informer.
type schedulingEventReader struct {
	client   kubernetes.Interface
	limiter  *rate.Limiter
	slots    chan struct{}
	flight   singleflight.Group
	mutex    sync.Mutex
	entries  map[string]schedulingEventEntry
	order    []string
	bytes    int
	now      func() time.Time
	timeout  time.Duration
	maxBytes int
}

func newSchedulingEventReader(client kubernetes.Interface) *schedulingEventReader {
	return &schedulingEventReader{
		client: client, limiter: rate.NewLimiter(1, 2), slots: make(chan struct{}, 2),
		entries: make(map[string]schedulingEventEntry), now: time.Now,
		timeout: schedulingEventTimeout, maxBytes: schedulingEventCacheBytes,
	}
}

func (r *schedulingEventReader) cached(key string) (biz.SchedulingEvents, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	entry, ok := r.entries[key]
	if !ok || !r.now().Before(entry.expires) {
		return biz.SchedulingEvents{}, false
	}
	return entry.value, true
}

func (r *schedulingEventReader) remember(key string, value biz.SchedulingEvents) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > r.maxBytes {
		return
	}
	ttl := schedulingEventTTL
	if value.Status != "available" && value.Status != "empty" {
		ttl = schedulingEventErrorTTL
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if old, ok := r.entries[key]; ok {
		r.bytes -= old.bytes
		delete(r.entries, key)
		for i, name := range r.order {
			if name == key {
				r.order = append(r.order[:i], r.order[i+1:]...)
				break
			}
		}
	}
	for len(r.order) > 0 && (len(r.entries) >= schedulingEventCacheItems || r.bytes+len(encoded) > r.maxBytes) {
		oldest := r.order[0]
		r.order = r.order[1:]
		r.bytes -= r.entries[oldest].bytes
		delete(r.entries, oldest)
	}
	r.entries[key] = schedulingEventEntry{value: value, expires: r.now().Add(ttl), bytes: len(encoded)}
	r.order = append(r.order, key)
	r.bytes += len(encoded)
}

func (r *schedulingEventReader) get(ctx context.Context, namespace, uid string) (biz.SchedulingEvents, error) {
	if err := ctx.Err(); err != nil {
		return biz.SchedulingEvents{}, err
	}
	key := namespace + "/" + uid
	if value, ok := r.cached(key); ok {
		return value, nil
	}
	result := r.flight.DoChan(key, func() (interface{}, error) {
		if value, ok := r.cached(key); ok {
			return value, nil
		}
		// Callers for one UID share a fetch that outlives any caller's cancel.
		select {
		case r.slots <- struct{}{}:
			defer func() { <-r.slots }()
		default:
			value := biz.SchedulingEvents{Status: "limited", Incomplete: true, FetchedAt: r.now()}
			r.remember(key, value)
			return value, nil
		}
		queryCtx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()
		value := r.read(queryCtx, namespace, uid)
		r.remember(key, value)
		return value, nil
	})
	select {
	case <-ctx.Done():
		return biz.SchedulingEvents{}, ctx.Err()
	case reply := <-result:
		if reply.Err != nil {
			return biz.SchedulingEvents{}, reply.Err
		}
		return reply.Val.(biz.SchedulingEvents), nil
	}
}

func (r *schedulingEventReader) read(ctx context.Context, namespace, uid string) biz.SchedulingEvents {
	result := biz.SchedulingEvents{Status: "available"}
	options := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("involvedObject.uid", uid).String(), Limit: schedulingEventPageSize}
	for page := 0; page < schedulingEventMaxPages; page++ {
		if err := r.limiter.Wait(ctx); err != nil {
			result.Status, result.Incomplete = "timeout", true
			break
		}
		events, err := r.client.CoreV1().Events(namespace).List(ctx, options)
		if err != nil {
			result.Status, result.Incomplete = schedulingEventErrorStatus(err), true
			break
		}
		for i := range events.Items {
			event := &events.Items[i]
			if string(event.InvolvedObject.UID) != uid || event.Namespace != namespace {
				continue
			}
			result.Items = append(result.Items, normalizeSchedulingEvent(event))
		}
		options.Continue = events.Continue
		if options.Continue == "" {
			break
		}
		if page == schedulingEventMaxPages-1 {
			result.Incomplete = true
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if !a.LastObservedAt.Equal(b.LastObservedAt) {
			return a.LastObservedAt.After(b.LastObservedAt)
		}
		return a.UID < b.UID
	})
	if len(result.Items) > schedulingEventMaxItems {
		result.Items = append([]biz.SchedulingEvent(nil), result.Items[:schedulingEventMaxItems]...)
		result.Incomplete = true
	}
	if len(result.Items) == 0 && result.Status == "available" {
		result.Status = "empty"
	}
	result.FetchedAt = r.now()
	return result
}

func schedulingEventErrorStatus(err error) string {
	switch {
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return "forbidden"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	case apierrors.IsTooManyRequests(err):
		return "limited"
	default:
		return "error"
	}
}

func normalizeSchedulingEvent(event *corev1.Event) biz.SchedulingEvent {
	message, truncated := boundedSchedulingText(event.Message, schedulingEventMaxMessage)
	result := biz.SchedulingEvent{
		UID: string(event.UID), Reason: event.Reason, Type: event.Type,
		Message: message, Source: event.Source.Component, Count: event.Count, MessageTruncated: truncated,
	}
	if result.Source == "" {
		result.Source = event.ReportingController
	}
	switch {
	case event.Series != nil && !event.Series.LastObservedTime.IsZero():
		result.LastObservedAt = event.Series.LastObservedTime.Time
	case !event.LastTimestamp.IsZero():
		result.LastObservedAt = event.LastTimestamp.Time
	case !event.EventTime.IsZero():
		result.LastObservedAt = event.EventTime.Time
	case !event.FirstTimestamp.IsZero():
		result.LastObservedAt = event.FirstTimestamp.Time
	default:
		result.LastObservedAt = event.CreationTimestamp.Time
	}
	if event.Series != nil {
		result.Count = event.Series.Count
	}
	if result.Count < 1 {
		result.Count = 1
	}
	return result
}
