package mail

// parseReport is what parsing had to salvage. Its caller logs it (Task 8);
// the tree and lists stay so the log can name the message's structure
// without its content.
type parseReport struct {
	Warnings []parseWarning
	root     *mimePart
	lists    bodyLists
	msg      *storedMessage
}
