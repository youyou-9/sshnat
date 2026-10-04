package version

import "testing"

func TestCurrentNormalizesBuildValues(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "local default", in: "dev", want: "dev"},
		{name: "release tag", in: "v1.2.3", want: "1.2.3"},
		{name: "whitespace", in: "  2.0.0  ", want: "2.0.0"},
		{name: "empty", in: "", want: "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.in
			if got := Current(); got != tc.want {
				t.Fatalf("Current() = %q, want %q", got, tc.want)
			}
		})
	}
}
