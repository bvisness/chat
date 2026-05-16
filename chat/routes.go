package chat

// Routes are defined here so it's easier to find them by opening a single file.

var Routes = []Route{
	{"GET", "/api/events", hEventStream},
}
