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

static void set_window_icon(GtkWindow *window, const unsigned char *data, int len) {
	GInputStream *stream = g_memory_input_stream_new_from_data(data, len, NULL);
	GdkPixbuf *pixbuf = gdk_pixbuf_new_from_stream(stream, NULL, NULL);
	if (pixbuf) {
		gtk_window_set_icon(window, pixbuf);
		g_object_unref(pixbuf);
	}
	g_object_unref(stream);
}

static void run_webview(const char* title, const char* url, int w, int h,
                        const unsigned char* icon_data, int icon_len) {
	gtk_init(NULL, NULL);
	GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(window), title);
	gtk_window_set_default_size(GTK_WINDOW(window), w, h);
	g_signal_connect(window, "destroy", G_CALLBACK(gtk_main_quit), NULL);

	if (icon_data && icon_len > 0) {
		set_window_icon(GTK_WINDOW(window), icon_data, icon_len);
	}

	WebKitWebView *webview = WEBKIT_WEB_VIEW(webkit_web_view_new());
	gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(webview));
	webkit_web_view_load_uri(webview, url);

	gtk_widget_show_all(window);
	gtk_main();
}

#elif defined(__APPLE__)
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static void run_webview(const char* title, const char* url, int w, int h,
                        const unsigned char* icon_data, int icon_len) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];

		if (icon_data && icon_len > 0) {
			NSData *data = [NSData dataWithBytes:icon_data length:icon_len];
			NSImage *icon = [[NSImage alloc] initWithData:data];
			if (icon) {
				[NSApp setApplicationIconImage:icon];
			}
		}

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
import (
	_ "embed"
	"os"
	"unsafe"
)

//go:embed icon.png
var iconPNG []byte

func init() {
	// WebKit's JavaScriptCore uses SIGUSR1 (signal 10) for GC by default,
	// which conflicts with Go's runtime signal handling and causes crashes.
	// Redirect JSC to use SIGUSR2 (signal 12) instead.
	os.Setenv("JSC_SIGNAL_FOR_GC", "12")
}

func openGUI(url string) error {
	title := C.CString("websessions")
	defer C.free(unsafe.Pointer(title))
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))

	var iconPtr *C.uchar
	iconLen := C.int(len(iconPNG))
	if len(iconPNG) > 0 {
		iconPtr = (*C.uchar)(unsafe.Pointer(&iconPNG[0]))
	}

	C.run_webview(title, curl, 1280, 800, iconPtr, iconLen)
	return nil
}
