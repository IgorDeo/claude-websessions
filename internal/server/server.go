package server

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/internal/store"
	"github.com/igor-deoalves/websessions/web"
)

type Server struct {
	cfg     *config.Config
	mgr     *session.Manager
	bus     *notification.Bus
	sink    *notification.InAppSink
	store   *store.Store
	hub     *wsHub
	handler http.Handler
}

func New(cfg *config.Config, mgr *session.Manager, bus *notification.Bus, sink *notification.InAppSink, st ...*store.Store) *Server {
	s := &Server{cfg: cfg, mgr: mgr, bus: bus, sink: sink, hub: newWSHub()}
	if len(st) > 0 {
		s.store = st[0]
	}
	s.handler = s.routes()
	s.setupNotificationBridge()
	mgr.OnOutput(func(sessionID string, data []byte) { s.hub.broadcast(sessionID, data) })
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	// Static files (no auth)
	staticFS, _ := fs.Sub(web.Static, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	// All other routes in auth group
	r.Group(func(r chi.Router) {
		if s.cfg.Auth.Enabled && s.cfg.Auth.Token != "" {
			r.Use(authMiddleware(s.cfg.Auth.Token))
		}
		r.Get("/", s.handleIndex)
		r.Get("/sidebar", s.handleSidebar)
		r.Get("/notifications", s.handleNotifications)
		r.Get("/api/dirs", s.handleListDirs)
		r.Get("/api/recent", s.handleRecentProjects)
		r.Get("/api/sessions", s.handleListSessions)
		r.Get("/sessions/new", s.handleNewSessionModal)
		r.Post("/sessions", s.handleCreateSession)
		r.Post("/sessions/{sessionID}/open", func(w http.ResponseWriter, r *http.Request) {
			s.handleOpenSession(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Post("/sessions/{sessionID}/rename", func(w http.ResponseWriter, r *http.Request) {
			s.handleRenameSession(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Post("/sessions/{sessionID}/kill", func(w http.ResponseWriter, r *http.Request) {
			s.handleKillSession(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Post("/sessions/{sessionID}/restart", func(w http.ResponseWriter, r *http.Request) {
			s.handleRestartSession(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Post("/sessions/{sessionID}/takeover", func(w http.ResponseWriter, r *http.Request) {
			s.handleTakeover(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Get("/ws/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
			s.handleWS(w, r, chi.URLParam(r, "sessionID"), s.mgr)
		})
	})
	return r
}
