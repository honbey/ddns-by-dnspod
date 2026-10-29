package model

import "testing"

func TestValidateDomain(t *testing.T) {
	valid := []string{
		"example.com",
		"sub.example.com",
		"my-domain.co.uk",
		"a.io",
		"xn--fiqs8s.asia",
	}
	for _, d := range valid {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", d, err)
		}
	}

	invalid := []struct {
		name   string
		domain string
	}{
		{"empty", ""},
		{"single label", "example"},
		{"trailing dot label", "example."},
		{"double dot", "exa..mple.com"},
		{"leading hyphen label", "-example.com"},
		{"trailing hyphen label", "example-.com"},
		{"underscore", "exa_mple.com"},
		{"space", "exa mple.com"},
		{"slash", "example.com/path"},
		{"script tag", "<script>alert(1)</script>.com"},
		{"img onerror", "<img src=x onerror=alert(1)>.com"},
		{"newline", "example\n.com"},
		{"leading dot", ".example.com"},
	}
	for _, c := range invalid {
		if err := ValidateDomain(c.domain); err == nil {
			t.Errorf("ValidateDomain(%q) [%s] must be rejected", c.domain, c.name)
		}
	}
}

func TestValidateSubDomain(t *testing.T) {
	valid := []string{
		"@",
		"*",
		"www",
		"a-b",
		"code-3000",
		"for_edgeone",
		"*.example",
		"deep.nested.host",
	}
	for _, s := range valid {
		if err := ValidateSubDomain(s); err != nil {
			t.Errorf("ValidateSubDomain(%q) = %v, want nil", s, err)
		}
	}

	invalid := []struct {
		name string
		sub  string
	}{
		{"empty", ""},
		{"space", "my host"},
		{"slash", "a/b"},
		{"script", "<script>x</script>"},
		{"quote", `a"onerror="alert(1)`},
		{"newline", "a\nb"},
		{"angle bracket", "a<b"},
		{"leading hyphen", "-host"},
		{"double dot", "a..b"},
	}
	for _, c := range invalid {
		if err := ValidateSubDomain(c.sub); err == nil {
			t.Errorf("ValidateSubDomain(%q) [%s] must be rejected", c.sub, c.name)
		}
	}
}

// The whole point of these rules is that stored names are inert wherever they
// end up. Make that explicit for the two sinks that motivated the validation.
func TestValidatedNamesCannotContainMarkup(t *testing.T) {
	payloads := []string{
		"<script>alert(1)</script>.com",
		"<img src=x onerror=alert(1)>.com",
		"example.com\nFAKE LOG LINE",
		"a\"onmouseover=\"alert(1)",
	}
	for _, p := range payloads {
		if err := ValidateDomain(p); err == nil {
			t.Errorf("domain payload %q must be rejected", p)
		}
		if err := ValidateSubDomain(p); err == nil {
			t.Errorf("subdomain payload %q must be rejected", p)
		}
	}
}

func TestValidateDomainLengthLimit(t *testing.T) {
	// 4 labels of 63 chars plus dots exceeds the cap
	long := ""
	for i := 0; i < 5; i++ {
		if i > 0 {
			long += "."
		}
		for j := 0; j < 63; j++ {
			long += "a"
		}
	}
	if err := ValidateDomain(long); err == nil {
		t.Error("an over-long domain must be rejected")
	}
}
