package branchname

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Add login button", "add-login-button"},
		{"  Fix: the Bug!! ", "fix-the-bug"},
		{"Hello___World", "hello-world"},
		{"", ""},
		{"!!!", ""},
		{"CamelCase 123", "camelcase-123"},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugify_CapsLength(t *testing.T) {
	long := ""
	for i := 0; i < 30; i++ {
		long += "ab "
	}
	got := Slugify(long)
	if len(got) > maxSlugLen {
		t.Errorf("slug length = %d, want <= %d", len(got), maxSlugLen)
	}
}

func TestGenerate(t *testing.T) {
	cases := []struct {
		name                       string
		template, title, id, tkey  string
		want                       string
	}{
		{"feature template", "feature/{slug}", "Add login", "id1234567890", "normal", "feature/add-login"},
		{"bug template", "bug/{slug}", "Fix crash", "abcdefgh1234", "bug", "bug/fix-crash"},
		{"empty title falls back to shortid", "bug/{slug}", "", "abcdefgh1234", "bug", "bug/abcdefgh"},
		{"shortid + type placeholders", "{type}/{slug}-{shortid}", "Hi", "abcdefgh1234", "hotfix", "hotfix/hi-abcdefgh"},
		{"empty template uses default", "", "Hi There", "abcdefgh1234", "release", "release/hi-there"},
		{"full id placeholder", "x/{id}", "t", "abcdefgh1234", "normal", "x/abcdefgh1234"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Generate(c.template, c.title, c.id, c.tkey); got != c.want {
				t.Errorf("Generate(%q, %q, %q, %q) = %q, want %q",
					c.template, c.title, c.id, c.tkey, got, c.want)
			}
		})
	}
}
