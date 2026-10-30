package mail

import (
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/appstate"
	"tinycld.org/core/syscfg"
)

// testSettingsKey holds the values a test feeds one app's syscfg seam. Kept on
// the app so helpers that each add a key (a server token, a stand-in API URL)
// compose instead of replacing one another's resolver.
var testSettingsKey = appstate.NewKey[*testSettings]("mail.test.settings")

type testSettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *testSettings) get(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key]
}

// setTestSetting sets key on app's syscfg seam for the rest of the test. The
// first call points the seam at the app's map; later calls add to it.
func setTestSetting(app core.App, key, value string) {
	s, ok := testSettingsKey.GetOk(app)
	if !ok {
		s = &testSettings{values: map[string]string{}}
		testSettingsKey.Set(app, s)
		syscfg.SetResolver(app, s.get)
	}
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
}

// newBareApp returns a plain test app for a test that needs an app only to
// carry its settings seam.
func newBareApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}
