package acme

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	xacme "golang.org/x/crypto/acme"
)

// failureReason is what the certificate row and the notification carry, so it
// has to say which step failed in words that can be acted on - and it must not
// carry the underlying error, which is what the log is for.
func TestFailureReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "a name no zone of the account serves",
			err:  &noZoneError{name: "_acme-challenge.a.example.com"},
			want: "no domain of this account serves _acme-challenge.a.example.com",
		},
		{
			name: "a challenge the CA could not validate",
			err:  &invalidChallengeError{name: "example.com", err: errors.New("boom")},
			want: "could not validate example.com",
		},
		{
			name: "a rate limit, by status",
			err:  &xacme.Error{StatusCode: http.StatusTooManyRequests},
			want: "rate limit",
		},
		{
			name: "a rate limit, by type",
			err:  &xacme.Error{ProblemType: "urn:ietf:params:acme:error:rateLimited"},
			want: "rate limit",
		},
		{
			name: "a name the CA refuses to issue",
			err:  &xacme.Error{ProblemType: "urn:ietf:params:acme:error:rejectedIdentifier"},
			want: "refused one of these names",
		},
		{
			name: "registration",
			err:  fmt.Errorf("%w: %v", errAccount, errors.New("boom")),
			want: "could not be registered",
		},
		{
			name: "publishing the record",
			err:  fmt.Errorf("%w: %v", errChallengePublish, errors.New("boom")),
			want: "could not be published",
		},
		{
			name: "the order",
			err:  fmt.Errorf("%w: %v", errOrder, errors.New("boom")),
			want: "refused the order",
		},
		{
			name: "running out of time",
			err:  context.DeadlineExceeded,
			want: "ran out of time",
		},
		{
			name: "something else",
			err:  errors.New("boom"),
			want: "see the log",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := failureReason(tc.err)
			if !strings.Contains(got, tc.want) {
				t.Errorf("failureReason(%v) = %q, want it to contain %q", tc.err, got, tc.want)
			}
			if strings.Contains(got, "boom") {
				t.Errorf("failureReason leaked the underlying error: %q", got)
			}
			if got == "" {
				t.Errorf("failureReason(%v) = %q, want something to show", tc.err, got)
			}
		})
	}
}

// What the CA said is more useful than the step it was said at: a rate limit is
// a rate limit whether the order was being created or confirmed, and it is the
// one answer that tells the reader to wait rather than to look at their DNS.
func TestFailureReasonPrefersWhatTheCASaid(t *testing.T) {
	problem := &xacme.Error{
		StatusCode:  http.StatusTooManyRequests,
		ProblemType: "urn:ietf:params:acme:error:rateLimited",
		Detail:      "too many certificates already issued for example.com",
	}
	err := fmt.Errorf("%w: %w", errOrder, problem)

	if got := failureReason(err); !strings.Contains(got, "rate limit") {
		t.Fatalf("failureReason = %q, want the rate limit sentence", got)
	}

	// And the step is still there for the log, which is where the CA's own
	// wording belongs.
	if !strings.Contains(err.Error(), errOrder.Error()) ||
		!strings.Contains(err.Error(), problem.Detail) {
		t.Fatalf("the wrapped error lost its parts: %v", err)
	}
}

func TestDNSChallengePicksTheRightOne(t *testing.T) {
	authz := &xacme.Authorization{Challenges: []*xacme.Challenge{
		{Type: "http-01", Token: "http"},
		{Type: "dns-01", Token: "dns"},
		{Type: "tls-alpn-01", Token: "tls"},
	}}
	challenge := dnsChallenge(authz)
	if challenge == nil || challenge.Token != "dns" {
		t.Fatalf("dnsChallenge = %+v, want the dns-01 challenge", challenge)
	}

	empty := &xacme.Authorization{Challenges: []*xacme.Challenge{{Type: "http-01"}}}
	if dnsChallenge(empty) != nil {
		t.Fatal("want no challenge when the CA offers no DNS-01 one")
	}
}
