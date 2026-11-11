package rules

import "testing"

func TestPushDenied(t *testing.T) {
	feature := PushContext{Current: "cursor/connect-everywhere-f24d", Default: "main"}
	onMain := PushContext{Current: "main", Default: "main"}
	unknown := PushContext{}
	trackedMain := PushContext{Current: "feature", Upstream: "main", Default: "main"}
	develop := PushContext{Current: "feature", Default: "develop"}
	withTag := PushContext{Current: "feature", Default: "main", Tags: map[string]bool{"v1.0.0": true}}

	deny := []struct {
		cmd string
		ctx PushContext
	}{
		{"git push origin main", feature},
		{"git push origin HEAD:main", feature},
		{"git push origin feature:master", feature},
		{"git push origin refs/heads/main", feature},
		{"git push origin feature:refs/heads/main", feature},
		{"git push", onMain},
		{"git push -u origin HEAD", onMain},
		{"git push", unknown},
		{"git push origin", unknown},
		{"git push", trackedMain},
		{"git push origin develop", develop},
		{"git push --force origin feature", feature},
		{"git push -f origin feature", feature},
		{"git push --force-with-lease origin feature", feature},
		{"git push origin +feature", feature},
		{"git push --tags", feature},
		{"git push origin --follow-tags feature", feature},
		{"git push origin refs/tags/v1", feature},
		{"git push origin v1.0.0", withTag},
		{"git push --all", feature},
		{"git push --mirror", feature},
		{"git push origin --delete main", feature},
		{"git push origin :main", feature},
		{"git status && git push origin main", feature},
		{`sh -c "git push origin main"`, feature},
		{"sudo git push origin main", feature},
	}
	allow := []struct {
		cmd string
		ctx PushContext
	}{
		{"git push -u origin HEAD", feature},
		{"git push -u origin cursor/connect-everywhere-f24d", feature},
		{"git push origin feature", unknown},
		{"git push origin feature:other", onMain},
		{"git push --no-follow-tags origin feature", feature},
		{"git status", feature},
		{"echo git push", feature},
		{"git push origin --delete feature", feature},
	}
	for _, tc := range deny {
		if !PushDenied(tc.cmd, tc.ctx) {
			t.Errorf("expected deny: %q", tc.cmd)
		}
	}
	for _, tc := range allow {
		if PushDenied(tc.cmd, tc.ctx) {
			t.Errorf("expected allow: %q", tc.cmd)
		}
	}
}
