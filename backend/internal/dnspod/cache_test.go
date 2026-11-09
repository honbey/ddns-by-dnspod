package dnspod

import (
	"errors"
	"testing"
	"time"

	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

func TestCacheSetAndGet(t *testing.T) {
	c := newCache[string](time.Minute)
	c.set("k", "v")

	got, ok := c.get("k")
	if !ok || got != "v" {
		t.Fatalf("want cached value, got %q ok=%v", got, ok)
	}
}

func TestCacheMissBeforeSet(t *testing.T) {
	c := newCache[string](time.Minute)
	if _, ok := c.get("missing"); ok {
		t.Fatal("want cache miss before any set")
	}
}

func TestCacheExpires(t *testing.T) {
	c := newCache[string](10 * time.Millisecond)
	c.set("k", "v")
	time.Sleep(20 * time.Millisecond)

	if _, ok := c.get("k"); ok {
		t.Fatal("want expired entry to be a miss")
	}
}

func TestCacheDisabledByZeroTTL(t *testing.T) {
	c := newCache[string](0)
	c.set("k", "v")

	if _, ok := c.get("k"); ok {
		t.Fatal("want zero TTL to disable caching")
	}
}

func TestCacheInvalidateByPrefix(t *testing.T) {
	c := newCache[[]int](time.Minute)
	c.set("example.com|a", []int{1})
	c.set("example.com|b", []int{2})
	c.set("other.com|a", []int{3})

	c.invalidate("example.com|")

	if _, ok := c.get("example.com|a"); ok {
		t.Error("example.com|a should be invalidated")
	}
	if _, ok := c.get("example.com|b"); ok {
		t.Error("example.com|b should be invalidated")
	}
	if _, ok := c.get("other.com|a"); !ok {
		t.Error("other.com|a should survive")
	}
}

func TestCacheInvalidateAll(t *testing.T) {
	c := newCache[string](time.Minute)
	c.set("a", "1")
	c.set("b", "2")

	c.invalidate("")

	if _, ok := c.get("a"); ok {
		t.Error("a should be gone")
	}
	if _, ok := c.get("b"); ok {
		t.Error("b should be gone")
	}
}

func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"code", errors.New("[TencentCloudSDKError] Code=RequestLimitExceeded"), true},
		{"typed error", sdkerrors.NewTencentCloudSDKError("RequestLimitExceeded", "request is limited", "rid"), true},
		{"chinese", errors.New("请求频率超过限制"), true},
		// A bare 429 in arbitrary text is not the provider's verdict: a webhook
		// that answered 429, or a domain and a value that happen to carry those
		// digits, never hit the DNSPod rate limit.
		{"webhook status is not a DNSPod limit", errors.New("ntfy returned status 429"), false},
		{"domain carrying 429", errors.New("list records (429.example.com): no such domain"), false},
		{"value carrying 429", errors.New("modify record: value ttl:429 invalid"), false},
		{"typed error with another code", sdkerrors.NewTencentCloudSDKError("InvalidParameter.RecordIdInvalid", "no such record", "rid"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRateLimited(tc.err); got != tc.want {
				t.Fatalf("IsRateLimited(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsRecordNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated error", errors.New("boom"), false},
		{"rate limit is not not-found", errors.New("[TencentCloudSDKError] Code=RequestLimitExceeded"), false},
		// The shape returned by the live API when a record id does not exist
		{"live delete of a missing id", errors.New("删除记录失败 (recordID=999999999999): [TencentCloudSDKError] Code=InvalidParameter.RecordIdInvalid, Message=记录编号错误。"), true},
		{"code only", errors.New("[TencentCloudSDKError] Code=InvalidParameter.RecordIdInvalid"), true},
		{"chinese message only", errors.New("记录不存在"), true},
		{"resource not found", errors.New("[TencentCloudSDKError] Code=ResourceNotFound.NoDataOfRecord"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRecordNotFound(tc.err); got != tc.want {
				t.Fatalf("IsRecordNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestDefaultCacheTTLIsUsable(t *testing.T) {
	// The defaults must stay positive, otherwise caching silently turns itself off.
	if DefaultCacheTTL.Domains <= 0 || DefaultCacheTTL.Records <= 0 || DefaultCacheTTL.Lines <= 0 {
		t.Fatalf("default TTLs must be positive, got %+v", DefaultCacheTTL)
	}
	if DefaultCacheTTL.Records >= DefaultCacheTTL.Domains {
		t.Fatalf("record TTL should be shorter than domain TTL, got %+v", DefaultCacheTTL)
	}
}
