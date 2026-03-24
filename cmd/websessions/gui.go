//go:build gui

package main

import webview "github.com/webview/webview_go"

func openGUI(url string) error {
	w := webview.New(true)
	defer w.Destroy()
	w.SetTitle("websessions")
	w.SetSize(1280, 800, webview.HintNone)
	w.Navigate(url)
	w.Run()
	return nil
}
