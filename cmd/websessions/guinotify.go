package main

// guiNotifyFunc is set by gui.go init() when built with the gui tag.
// When non-nil, it sends notifications through the native GTK/Cocoa
// notification system. Clicking the notification focuses the window.
var guiNotifyFunc func(title, body, id string)
