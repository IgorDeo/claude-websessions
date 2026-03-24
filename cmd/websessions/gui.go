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

static GtkWidget *main_window = NULL;
static GApplication *app = NULL;

static void set_window_icon(GtkWindow *window, const unsigned char *data, int len) {
	GInputStream *stream = g_memory_input_stream_new_from_data(data, len, NULL);
	GdkPixbuf *pixbuf = gdk_pixbuf_new_from_stream(stream, NULL, NULL);
	if (pixbuf) {
		gtk_window_set_icon(window, pixbuf);
		g_object_unref(pixbuf);
	}
	g_object_unref(stream);
}

static void on_activate(GtkApplication *gtkapp, gpointer user_data) {
	if (main_window) {
		gtk_window_present(GTK_WINDOW(main_window));
	}
}

typedef struct {
	char *title;
	char *body;
	char *id;
} NotifData;

static gboolean send_notif_idle(gpointer data) {
	NotifData *nd = (NotifData *)data;
	if (app) {
		GNotification *notif = g_notification_new(nd->title);
		g_notification_set_body(notif, nd->body);
		g_notification_set_default_action(notif, "app.activate");
		g_application_send_notification(app, nd->id, notif);
		g_object_unref(notif);
	}
	free(nd->title);
	free(nd->body);
	free(nd->id);
	free(nd);
	return G_SOURCE_REMOVE;
}

void gui_send_notification(const char *title, const char *body, const char *id) {
	NotifData *nd = malloc(sizeof(NotifData));
	nd->title = strdup(title);
	nd->body = strdup(body);
	nd->id = strdup(id);
	g_idle_add(send_notif_idle, nd);
}

static void app_activate(GtkApplication *gtkapp, gpointer user_data) {
	(void)user_data;
	const char **params = (const char **)g_object_get_data(G_OBJECT(gtkapp), "params");
	const char *title = params[0];
	const char *url = params[1];
	int w = GPOINTER_TO_INT(params[2]);
	int h = GPOINTER_TO_INT(params[3]);
	const unsigned char *icon_data = (const unsigned char *)params[4];
	int icon_len = GPOINTER_TO_INT(params[5]);

	main_window = gtk_application_window_new(gtkapp);
	gtk_window_set_title(GTK_WINDOW(main_window), title);
	gtk_window_set_default_size(GTK_WINDOW(main_window), w, h);

	if (icon_data && icon_len > 0) {
		set_window_icon(GTK_WINDOW(main_window), icon_data, icon_len);
	}

	WebKitWebView *webview = WEBKIT_WEB_VIEW(webkit_web_view_new());
	gtk_container_add(GTK_CONTAINER(main_window), GTK_WIDGET(webview));
	webkit_web_view_load_uri(webview, url);

	gtk_widget_show_all(main_window);
}

static void run_webview(const char* title, const char* url, int w, int h,
                        const unsigned char* icon_data, int icon_len) {
	GtkApplication *gtkapp = gtk_application_new("com.websessions.app",
		G_APPLICATION_FLAGS_NONE);
	app = G_APPLICATION(gtkapp);

	const void **params = malloc(sizeof(void*) * 6);
	params[0] = title;
	params[1] = url;
	params[2] = GINT_TO_POINTER(w);
	params[3] = GINT_TO_POINTER(h);
	params[4] = icon_data;
	params[5] = GINT_TO_POINTER(icon_len);
	g_object_set_data(G_OBJECT(gtkapp), "params", params);

	g_signal_connect(gtkapp, "activate", G_CALLBACK(app_activate), NULL);

	g_application_run(app, 0, NULL);

	g_object_unref(gtkapp);
	free(params);
	app = NULL;
	main_window = NULL;
}

#elif defined(__APPLE__)
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static NSWindow *main_window_mac = nil;

void gui_send_notification(const char *title, const char *body, const char *id) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSUserNotification *notif = [[NSUserNotification alloc] init];
		notif.title = [NSString stringWithUTF8String:title];
		notif.informativeText = [NSString stringWithUTF8String:body];
		notif.identifier = [NSString stringWithUTF8String:id];
		[[NSUserNotificationCenter defaultUserNotificationCenter] deliverNotification:notif];
	});
}

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
		main_window_mac = [[NSWindow alloc]
			initWithContentRect:frame
			styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
			           NSWindowStyleMaskResizable | NSWindowStyleMaskMiniaturizable)
			backing:NSBackingStoreBuffered
			defer:NO];
		[main_window_mac setTitle:[NSString stringWithUTF8String:title]];
		[main_window_mac center];

		WKWebView *webview = [[WKWebView alloc] initWithFrame:frame];
		[main_window_mac setContentView:webview];
		[webview loadRequest:[NSURLRequest requestWithURL:
			[NSURL URLWithString:[NSString stringWithUTF8String:url]]]];

		[main_window_mac makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp run];
	}
}
#endif
*/
import "C"
import (
	_ "embed"
	"unsafe"
)

//go:embed icon.png
var iconPNG []byte

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

// guiNotify sends a native desktop notification via GTK/Cocoa.
// Clicking the notification brings the websessions window to focus.
func guiNotify(title, body, id string) {
	ct := C.CString(title)
	cb := C.CString(body)
	ci := C.CString(id)
	C.gui_send_notification(ct, cb, ci)
	C.free(unsafe.Pointer(ct))
	C.free(unsafe.Pointer(cb))
	C.free(unsafe.Pointer(ci))
}

func init() {
	guiNotifyFunc = guiNotify
}
