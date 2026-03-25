/*
 * webview-helper: standalone WebKit2GTK webview window.
 * Compiled as a separate binary to avoid Go runtime signal conflicts.
 *
 * Usage: webview-helper <url> [icon.png]
 */
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <stdio.h>
#include <string.h>

static void on_destroy(GtkWidget *widget, gpointer data) {
    gtk_main_quit();
}

static void set_icon_from_file(GtkWindow *window, const char *path) {
    GdkPixbuf *pixbuf = gdk_pixbuf_new_from_file(path, NULL);
    if (pixbuf) {
        gtk_window_set_icon(window, pixbuf);
        g_object_unref(pixbuf);
    }
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        fprintf(stderr, "usage: webview-helper <url> [icon.png]\n");
        return 1;
    }

    const char *url = argv[1];
    const char *icon_path = argc > 2 ? argv[2] : NULL;

    gtk_init(&argc, &argv);

    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "websessions");
    gtk_window_set_default_size(GTK_WINDOW(window), 1280, 800);
    g_signal_connect(window, "destroy", G_CALLBACK(on_destroy), NULL);

    if (icon_path) {
        set_icon_from_file(GTK_WINDOW(window), icon_path);
    }

    WebKitWebView *webview = WEBKIT_WEB_VIEW(webkit_web_view_new());
    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(webview));
    webkit_web_view_load_uri(webview, url);

    gtk_widget_show_all(window);
    gtk_main();

    return 0;
}
