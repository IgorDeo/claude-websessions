//go:build gui

package main

/*
#cgo linux pkg-config: gtk+-3.0 webkit2gtk-4.1
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework WebKit -framework Cocoa

#include <stdlib.h>

#if defined(__linux__)
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static void run_webview(const char* title, const char* url, int w, int h) {
	gtk_init(NULL, NULL);
	GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(window), title);
	gtk_window_set_default_size(GTK_WINDOW(window), w, h);
	g_signal_connect(window, "destroy", G_CALLBACK(gtk_main_quit), NULL);

	WebKitWebView *webview = WEBKIT_WEB_VIEW(webkit_web_view_new());
	gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(webview));
	webkit_web_view_load_uri(webview, url);

	gtk_widget_show_all(window);
	gtk_main();
}

#elif defined(__APPLE__)
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static void run_webview(const char* title, const char* url, int w, int h) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];

		NSRect frame = NSMakeRect(0, 0, w, h);
		NSWindow *window = [[NSWindow alloc]
			initWithContentRect:frame
			styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
			           NSWindowStyleMaskResizable | NSWindowStyleMaskMiniaturizable)
			backing:NSBackingStoreBuffered
			defer:NO];
		[window setTitle:[NSString stringWithUTF8String:title]];
		[window center];

		WKWebView *webview = [[WKWebView alloc] initWithFrame:frame];
		[window setContentView:webview];
		[webview loadRequest:[NSURLRequest requestWithURL:
			[NSURL URLWithString:[NSString stringWithUTF8String:url]]]];

		[window makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp run];
	}
}
#endif
*/
import "C"
import "unsafe"

func openGUI(url string) error {
	title := C.CString("websessions")
	defer C.free(unsafe.Pointer(title))
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))
	C.run_webview(title, curl, 1280, 800)
	return nil
}
