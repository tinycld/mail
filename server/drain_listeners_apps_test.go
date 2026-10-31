package mail

import "testing"

// Two apps in one process each keep the listeners their mail servers accept
// on. One app's drain must close only its own.
func TestAcceptingListenersArePerApp(t *testing.T) {
	appA, appB := newProdApp(t), newProdApp(t)

	lnA, err := acceptingListenersOf(appA).listen("imaps", ":993")("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lnA.Close() })
	lnB, err := acceptingListenersOf(appB).listen("imaps", ":993")("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lnB.Close() })

	acceptingListenersOf(appA).closeAll()

	if !refused(lnA.Addr().String()) {
		t.Fatal("app A's listener still accepts after app A's drain")
	}
	if refused(lnB.Addr().String()) {
		t.Fatal("app A's drain closed app B's listener")
	}
}
